package main

import (
  "fmt"
  "net/http"
  "html/template"
  "errors"
  "time"
  "strings"
  "sort"

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
var fileRead = false

func indexHandler(file *excelize.File, month *MonthEntry) http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
      tpl.Execute(w, *month)
      return
    }

    addNewEntry(file, month, r)

    tpl.Execute(w, *month)
  }
}

func addNewEntry(file *excelize.File, month *MonthEntry, r *http.Request) {
    // Add entry from form
    entryDate, err := time.Parse("2006-01-02", strings.TrimSpace(r.FormValue("date")))
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

    sortEntries(month)

    // Add entry to actual file, at the bottom of the file
    rows, err := file.GetRows("jan2021")
    if err != nil {
        fmt.Println(err)
    }

    lastRow := len(rows)
    insertErr := file.InsertRow("jan2021", lastRow)
    if insertErr != nil {
        fmt.Println(insertErr)
    }
    file.SetCellValue("jan2021", fmt.Sprintf("A%d", lastRow), entry.Date.Format("02/01/06"))
    file.SetCellValue("jan2021", fmt.Sprintf("B%d", lastRow), entry.Category)
    if entry.Who == "A" {
      if entry.Currency == "EUR" {
        file.SetCellValue("jan2021", fmt.Sprintf("C%d", lastRow), entry.Quantity)
      } else if entry.Currency == "CHF" {
        file.SetCellValue("jan2021", fmt.Sprintf("D%d", lastRow), entry.Quantity)
      }
    } else if entry.Who == "P" {
      if entry.Currency == "EUR" {
        file.SetCellValue("jan2021", fmt.Sprintf("E%d", lastRow), entry.Quantity)
      } else if entry.Currency == "CHF" {
        file.SetCellValue("jan2021", fmt.Sprintf("F%d", lastRow), entry.Quantity)
      }
    } else if entry.Who == "B" {
      file.SetCellValue("jan2021", fmt.Sprintf("G%d", lastRow), entry.Quantity)
    }
    file.SetCellValue("jan2021", fmt.Sprintf("H%d", lastRow), entry.Comment)
}



func sortEntries(month *MonthEntry) {
  sort.SliceStable((*month).Entries, func(i, j int) bool {
    return (*month).Entries[i].Date.Before((*month).Entries[j].Date)
  })
}


func writeExcel(file *excelize.File, month *MonthEntry) http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {
    fmt.Println("Saving file")
    err := file.Save()
    if err != nil {
        fmt.Println(err)
    }

    tpl.Execute(w, *month)
  }
}


func readExcelFile(month *MonthEntry, path string) (MonthEntry, *excelize.File, error) {
  f, err := excelize.OpenFile(path)
  if err != nil {
    fmt.Println(err)
    return MonthEntry{}, f, errors.New("Could not open file")
  }

  rows, err := f.GetRows("jan2021")
  if err != nil {
      fmt.Println(err)
      return MonthEntry{}, f, errors.New("Error reading rows")
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
  (*month).MinDate = (*month).Entries[0].Date
  (*month).MaxDate = (*month).Entries[len((*month).Entries)-1].Date

  return *month, f, nil
}


func main() {
  port := "3000"

  fs := http.FileServer(http.Dir("assets"))

  // Read input file
  monthEntries := MonthEntry{}
  file := &excelize.File{}
  if fileRead == false {
    filePath := os.Args[1]
    fmt.Println("Reading excel file: ", filePath)
    me, f, err := readExcelFile(&monthEntries, filePath)
    file = f
    monthEntries = me
    if err != nil {
      fmt.Println(err)
    }
    sortEntries(&monthEntries)
    fileRead = true
  }

  mux := http.NewServeMux()

  mux.Handle("/assets/", http.StripPrefix("/assets/", fs))
  mux.HandleFunc("/writeExcel", writeExcel(file, &monthEntries))
  mux.HandleFunc("/", indexHandler(file, &monthEntries))
  http.ListenAndServe(":"+port, mux)
}
