package exchRates

import (
  "fmt"
  "net/http"
  "encoding/json"
  "io/ioutil"
  "os"
  "time"
  "errors"
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

func GetRate(from, to string, date time.Time) (float64, error) {

	if from != "CHF" {
		return 1.0, errors.New("Unsupported exchange rate. defaulting to 1.0 exchange rate")
	}

	oeid := os.Getenv("OPEN_EXCHANGE_APP_ID")
	if oeid == "" {
		return 1.0, errors.New("No OPEN_EXCHANGE_APP_ID env variable found, defaulting to 1.0 exchange rate")
	}
	symbols := fmt.Sprintf("%s,%s", from, to)
	const url_date_layout string = "2006-01-02"
	url_date := date.Format(url_date_layout)
	urlRequest := fmt.Sprintf("https://openexchangerates.org/api/historical/%s.json?app_id=%s&symbols=%s",
		url_date, oeid, symbols)
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

	return exData.Rates.Eur/exData.Rates.Chf, nil
}