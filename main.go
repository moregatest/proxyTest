// Exercise: Web Crawler - http://tour.golang.org/#70
// modified to use the worker pool
package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"mytool/libs/pool"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"time"

	"github.com/oschwald/geoip2-golang"
)

type Fetcher interface {
	// Fetch returns the body of URL and
	// a slice of URLs found on that page.
	Fetch(url string) (body string, urls []string, err error)
}

type crawlResult struct {
	code int
	err  error
}

func Pinger(domain string, timeoutSecond int) (int, error) {
	if "http" != domain[0:4] {
		domain = fmt.Sprintf("http://%v", domain)
	}
	proxyUrl, err := url.Parse(domain)
	if err != nil {
		return 0, err
	}
	client := &http.Client{
		Transport: &http.Transport{
			Dial:  (&net.Dialer{Timeout: time.Duration(timeoutSecond) * time.Second}).Dial,
			Proxy: http.ProxyURL(proxyUrl),
		},
		Timeout: 3 * time.Second,
	}
	testurl := os.Args[1]
	if testurl == "" {
		testurl = "https://www.google.com"
	}
	req, err := http.NewRequest("HEAD", testurl, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 6.2; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/27.0.1453.94 Safari/537.36")
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	resp.Body.Close()
	return resp.StatusCode, nil
}

// work uses fetcher to recursively crawl
// pages starting with url, to a maximum of depth.
func work(args ...interface{}) interface{} {
	url := args[0].(string)
	//timeout := args[1].(int)
	timeout := 10
	if url == "" {
		return crawlResult{}
	}
	code, err := Pinger(url, timeout)
	return crawlResult{code, err}
}

var mypool = pool.New(1000) // number of workers

func main() {
	regex := regexp.MustCompile(`([\S]+)`)
	cpus := runtime.NumCPU()
	runtime.GOMAXPROCS(cpus)
	mypool.Run()
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		log.Fatal(err)
	}
	db, err := geoip2.Open(dir + "/GeoLite2-City.mmdb")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	b, err := ioutil.ReadFile(dir + `/ip.txt`)
	if err != nil {
		return
	}
	proxys := regex.FindAllString(string(b), -1)
	//取得一次性擊發數目 預設5
	oneTimeconn := "5"
	if len(os.Args) == 3 {
		oneTimeconn = os.Args[2]
	}

	excludeCountryCode := ""
	if len(os.Args) == 4 {
		excludeCountryCode = os.Args[3]
	}

	for _, proxy := range proxys {

		tmpip := findIP(proxy)
		ip := net.ParseIP(tmpip)
		record, err := db.City(ip)
		if err != nil {
			log.Fatal(err)
		}
		//fmt.Printf("%s ISO country code: %v\n", tmpip, record.Country.IsoCode)
		//如果符合規避國家 即跳過不送出
		if excludeCountryCode == record.Country.IsoCode {
			//fmt.Printf("[%v]skin %v\n", excludeCountryCode, tmpip)
			continue
		}
		proxy := string(proxy)
		mypool.Add(work, proxy, oneTimeconn)
	}
	for {
		job := mypool.WaitForJob()
		if job == nil {
			break
		}
		if job.Result == nil {
			fmt.Println("got error:", job.Err)
		} else {
			result := job.Result.(crawlResult)
			if result.err == nil {
				url := job.Args[0].(string)
				fmt.Printf("%v\n", url)
			}
		}
		//fmt.Printf("%v\n", job)

	}
	mypool.Stop()
}

func findIP(input string) string {
	numBlock := "(25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])"
	regexPattern := numBlock + "\\." + numBlock + "\\." + numBlock + "\\." + numBlock

	regEx := regexp.MustCompile(regexPattern)
	return regEx.FindString(input)
}
