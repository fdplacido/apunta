package main

import (
	"fmt"
	"net/http"
	"html/template"
	// "net/url"
)

type Entry struct {
    Who      string
    Currency string
    Quantity string
}

var tpl = template.Must(template.ParseFiles("index.html"))

func indexHandler(entries []Entry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
	  if r.Method != http.MethodPost {
	    tpl.Execute(w, entries)
	    return
	  }

		entry := Entry{
			Who: r.FormValue("who"),
			Currency: r.FormValue("currency"),
			Quantity: r.FormValue("quantity"),
		}

		entries = append(entries, entry)

	  for _, myentry := range entries {
			fmt.Println("Who is: ", myentry.Who)
			fmt.Println("Currency is: ", myentry.Currency)
			fmt.Println("Quantity is: ", myentry.Quantity)
	  }

		tpl.Execute(w, entries)
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

	// Entries stuff
	myentries := make([]Entry, 0)
	myentries = append(myentries, genEntry());
	myentries = append(myentries, genEntry());
	myentries = append(myentries, genEntry());

	mux := http.NewServeMux()

	mux.Handle("/assets/", http.StripPrefix("/assets/", fs))
	mux.HandleFunc("/", indexHandler(myentries))
	http.ListenAndServe(":"+port, mux)
}
