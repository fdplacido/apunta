package exchRages

import (
  "fmt"
  "net/http"
  "encoding/json"
  "io/ioutil"
  "os"
)

type exchangeData struct {
	Disclaimer string `json:"disclaimer"`
	License    string `json:"license"`
	Timestamp  int    `json:"timestamp"`
	Base       string `json:"base"`
	Rates      struct {
		Chf float64 `json:"CHF"`
		Eur float64 `json:"EUR"`
	} `json:"rates"`
}

func GetRate(from, to string) float64 {
	oeid := os.Getenv("OPEN_EXCHANGE_APP_ID")
	symbols := fmt.Sprintf("%s,%s", from, to)
	urlRequest := fmt.Sprintf("https://openexchangerates.org/api/latest.json?app_id=%s&symbols=%s",
		oeid, symbols)
	resp, err := http.Get(urlRequest)
	if err != nil {
	   fmt.Println(err)
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
	  fmt.Println(err)
	}

	exData := &exchangeData{}
	json.Unmarshal(body, exData)

	return exData.Rates.Eur/exData.Rates.Chf
}