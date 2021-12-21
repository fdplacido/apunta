package main

import (
	"fmt"
	"net/http"
	"html/template"

	"os"

	"github.com/xuri/excelize/v2"
)

type Entry struct {
    Who      string
    Currency string
    Quantity string
    Comment  string
}

var tpl = template.Must(template.ParseFiles("index.html"))

func indexHandler(entries *[]Entry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
	  if r.Method != http.MethodPost {

		  filePath := os.Args[1]
			readExcelFile(entries, filePath)

	    tpl.Execute(w, *entries)
	    return
	  }

	  // Add entry from form
		entry := Entry{
			Who: r.FormValue("who"),
			Currency: r.FormValue("currency"),
			Quantity: r.FormValue("quantity"),
			Comment: r.FormValue("comment"),
		}
		*entries = append(*entries, entry)

		tpl.Execute(w, *entries)
	}
}



func readExcelFile(entries *[]Entry, path string) {
  f, err := excelize.OpenFile(path)
  if err != nil {
    fmt.Println(err)
    return
  }

	rows, err := f.GetRows("jan2021")
  if err != nil {
      fmt.Println(err)
      return
  }

  for index, row := range rows {

  	// Skip first row
  	if index == 0 {
  		continue
  	}

  	// Only process if there is something in the row
  	if len(row) == 0 {
  		continue
  	}

  	if row[2] != "" {
			entry := Entry {
				Who: "A",
				Currency: "EUR",
				Quantity: row[2],
				Comment: row[7],
			}
			*entries = append(*entries, entry);
  	} else if row[3] != "" {
			entry := Entry {
				Who: "A",
				Currency: "CHF",
				Quantity: row[3],
				Comment: row[7],
			}
			*entries = append(*entries, entry);
  	} else if row[4] != "" {
			entry := Entry {
				Who: "P",
				Currency: "EUR",
				Quantity: row[4],
				Comment: row[7],
			}
			*entries = append(*entries, entry);
  	} else if row[5] != "" {
			entry := Entry {
				Who: "P",
				Currency: "CHF",
				Quantity: row[5],
				Comment: row[7],
			}
			*entries = append(*entries, entry);
  	} else if row[6] != "" {
			entry := Entry {
				Who: "B",
				Currency: "EUR",
				Quantity: row[6],
				Comment: row[7],
			}
			*entries = append(*entries, entry);
  	}
  }
}


func main() {
	port := "3000"

	fs := http.FileServer(http.Dir("assets"))

	// Init empty entries
	myentries := make([]Entry, 0)

	mux := http.NewServeMux()

	mux.Handle("/assets/", http.StripPrefix("/assets/", fs))
	mux.HandleFunc("/", indexHandler(&myentries))
	http.ListenAndServe(":"+port, mux)
}
