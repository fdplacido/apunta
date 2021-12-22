package main

import (
  "fmt"
  "net/http"
  "html/template"
  "errors"
  "time"
  "strings"

  "os"

  "github.com/xuri/excelize/v2"
)

type Entry struct {
  Date     time.Time
  Category string
  Who      string
  Currency string
  Quantity string
  Comment  string
}

type MonthEntry struct {
  MaxDate time.Time
  MinDate time.Time
  Entries []Entry
}

var tpl = template.Must(template.ParseFiles("index.html"))

func indexHandler(month *MonthEntry) http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {

      filePath := os.Args[1]
      *month, _ = readExcelFile(month, filePath)

      tpl.Execute(w, *month)
      return
    }

    // Add entry from form
    entryDate, err := time.Parse("02/01/2006", r.FormValue("date"))
    if err != nil {
      fmt.Println(err)
      entryDate = time.Time{}
    }

    entry := Entry{
      Date: entryDate,
      Category: r.FormValue("category"),
      Who: r.FormValue("who"),
      Currency: r.FormValue("currency"),
      Quantity: r.FormValue("quantity"),
      Comment: r.FormValue("comment"),
    }
    (*month).Entries = append((*month).Entries, entry)

    tpl.Execute(w, *month)
  }
}


func readExcelFile(month *MonthEntry, path string) (MonthEntry, error) {
  f, err := excelize.OpenFile(path)
  if err != nil {
    fmt.Println(err)
    return MonthEntry{}, errors.New("Could not open file")
  }

  rows, err := f.GetRows("jan2021")
  if err != nil {
      fmt.Println(err)
      return MonthEntry{}, errors.New("Error reading rows")
  }

  entries := (*month).Entries

  for index, row := range rows {
    // Skip first row
    if index == 0 {
      continue
    }

    // Only process if there is something in the row
    if len(row) == 0 {
      continue
    }

    // Parse Dates
    const layout = "02/01/06"
    entryDate, err := time.Parse(layout, strings.TrimSpace(row[0]))
    if err != nil {
      fmt.Println(err)
      entryDate = time.Time{}
    }

    if row[2] != "" {
      entry := Entry {
        Date: entryDate,
        Category: row[1],
        Who: "A",
        Currency: "EUR",
        Quantity: row[2],
        Comment: row[7],
      }
      entries = append(entries, entry);
    } else if row[3] != "" {
      entry := Entry {
        Date: entryDate,
        Category: row[1],
        Who: "A",
        Currency: "CHF",
        Quantity: row[3],
        Comment: row[7],
      }
      entries = append(entries, entry);
    } else if row[4] != "" {
      entry := Entry {
        Date: entryDate,
        Category: row[1],
        Who: "P",
        Currency: "EUR",
        Quantity: row[4],
        Comment: row[7],
      }
      entries = append(entries, entry);
    } else if row[5] != "" {
      entry := Entry {
        Date: entryDate,
        Category: row[1],
        Who: "P",
        Currency: "CHF",
        Quantity: row[5],
        Comment: row[7],
      }
      entries = append(entries, entry);
    } else if row[6] != "" {
      entry := Entry {
        Date: entryDate,
        Category: row[1],
        Who: "B",
        Currency: "EUR",
        Quantity: row[6],
        Comment: row[7],
      }
      entries = append(entries, entry);
    }
  }

  (*month).Entries = entries

  // Get Max and Min
  (*month).MaxDate = (*month).Entries[0].Date
  (*month).MinDate = (*month).Entries[len((*month).Entries)-1].Date

  return *month, nil
}


func main() {
  port := "3000"

  fs := http.FileServer(http.Dir("assets"))

  // Init empty entries
  // myentries := make([]Entry, 0)
  monthEntries := MonthEntry{}

  mux := http.NewServeMux()

  mux.Handle("/assets/", http.StripPrefix("/assets/", fs))
  mux.HandleFunc("/", indexHandler(&monthEntries))
  http.ListenAndServe(":"+port, mux)
}
