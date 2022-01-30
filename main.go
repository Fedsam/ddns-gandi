package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"gopkg.in/yaml.v2"
)

type Config struct {
	URL             string   `yaml:"url"`
	IP_RESOLVER_URL string   `yaml:"ip-resolver-url"`
	DOMAIN          string   `yaml:"domain"`
	API_KEY         string   `yaml:"apikey"`
	SUBDOMAINS      []string `yaml:"subdomains"`
}

type Record []struct {
	RrsetName   string   `json:"rrset_name"`
	RrsetType   string   `json:"rrset_type"`
	RrsetTTL    int      `json:"rrset_ttl"`
	RrsetValues []string `json:"rrset_values"`
	RrsetHref   string   `json:"rrset_href"`
}

type PutRecord struct {
	RrsetType   string   `json:"rrset_type"`
	RrsetValues []string `json:"rrset_values"`
}

type PutData struct {
	Items []PutRecord `json:"items"`
}

func (c *Config) Parse(data []byte) error {
	return yaml.Unmarshal(data, c)
}

func initConfig() *Config {
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		log.Fatalln(err)
	}

	var config Config
	if err := config.Parse(data); err != nil {
		log.Fatalln(err)
	}

	return &config
}

func checkCurrentIP(c *Config, currentIP string) (bool, string) {
	res, err := http.Get(c.IP_RESOLVER_URL)
	if err != nil {
		log.Fatalln(err)
	}
	defer res.Body.Close()

	ipAddress, err := io.ReadAll(res.Body)
	if err != nil {
		log.Fatalln(err)
	}
	ipAddress = ipAddress[:len(ipAddress)-1]

	return string(ipAddress) == currentIP, string(ipAddress) // weird newline char added
}

func NewAPIRequest(c *Config, method string, url string) *http.Request {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		log.Fatalln(err)
	}
	req.Header.Add("Authorization", fmt.Sprintf("Apikey %s", c.API_KEY))
	return req
}

func NewAPIRequestWithBody(c *Config, method string, url string, body []byte) *http.Request {
	req := NewAPIRequest(c, method, url)

	req.Header.Add("Content-Type", "application/json; charset=UTF-8")
	req.Body = io.NopCloser(strings.NewReader(string(body)))

	return req
}

func callAPI(client *http.Client, req *http.Request) ([]byte, int) {
	res, err := client.Do(req)
	if err != nil {
		log.Fatalln(err)
	}

	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		log.Fatalln(err)
	}

	return body, res.StatusCode
}

func updateRecords(client *http.Client, c *Config, url string) bool {
	for _, subdomain := range c.SUBDOMAINS {
		var result Record
		currentUrl := fmt.Sprintf("%s/%s", url, subdomain)
		resBody, _ := callAPI(client, NewAPIRequest(c, http.MethodGet, currentUrl))

		if err := json.Unmarshal(resBody, &result); err != nil {
			log.Fatalln(err)
		}

		isSameIP, newIP := checkCurrentIP(c, result[0].RrsetValues[0])
		if isSameIP {
			log.Println(fmt.Sprintf("%s \t[OK]", subdomain))
			continue
		}

		putRecord := &PutRecord{RrsetType: "A", RrsetValues: []string{newIP}}
		putData := &PutData{Items: []PutRecord{*putRecord}}
		jsonRecord, err := json.Marshal(putData)
		if err != nil {
			log.Fatalln(err)
		}

		_, statusCode := callAPI(client, NewAPIRequestWithBody(c, http.MethodPut, currentUrl, jsonRecord))
		if statusCode != http.StatusCreated {
			return false
		}
		log.Println(fmt.Sprintf("%s \t[UPDATED]", subdomain))
	}
	return true
}

func main() {
	config := initConfig()
	client := &http.Client{}
	config.URL = strings.Replace(config.URL, "xxx", config.DOMAIN, -1)

	updateRecords(client, config, config.URL)
}
