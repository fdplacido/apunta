package main

import (
	"fmt"
	"net/http"
	"html/template"

	"github.com/xuri/excelize/v2"
)

type Entry struct {
    Who      string
    Currency string
    Quantity string
}

var tpl = template.Must(template.ParseFiles("index.html"))

func indexHandler(entries *[]Entry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
	  if r.Method != http.MethodPost {
	    tpl.Execute(w, *entries)
	    return
	  }

		readExcelFile(entries)

	  // Add entry from form
		entry := Entry{
			Who: r.FormValue("who"),
			Currency: r.FormValue("currency"),
			Quantity: r.FormValue("quantity"),
		}
		*entries = append(*entries, entry)

		tpl.Execute(w, *entries)
	}
}


func readExcelFile(entries *[]Entry) {
  f, err := excelize.OpenFile("")
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
			entry := Entry{
				Who: "A",
				Currency: "EUR",
				Quantity: row[2],
			}

			*entries = append(*entries, entry);
  	}
  }
}


func genEntry() Entry {
	entry := Entry{
		Who: "who",
		Currency: "currency",
		Quantity: "quantity",
	}
	return entry;
}

func main() {
	port := "3000"

	fs := http.FileServer(http.Dir("assets"))

	// Init dummy entries
	myentries := make([]Entry, 0)
	myentries = append(myentries, genEntry());

	mux := http.NewServeMux()

	mux.Handle("/assets/", http.StripPrefix("/assets/", fs))
	mux.HandleFunc("/", indexHandler(&myentries))
	http.ListenAndServe(":"+port, mux)
}
