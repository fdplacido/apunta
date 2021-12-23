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

type DayRec struct {
  Date     time.Time
  Category string
  Who      string
  Currency string
  Quantity string
  Comment  string
}

type MonthRec struct {
  MaxDate   time.Time
  MinDate   time.Time
  MonthName string
  DayRecords   []DayRec
}

var tpl = template.Must(template.ParseFiles("index.html"))
var inputFileRead = false

func indexHandler(file *excelize.File, month *MonthRec) http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
      tpl.Execute(w, *month)
      return
    }

    addNewEntry(file, month, r)

    tpl.Execute(w, *month)
  }
}

func addNewEntry(file *excelize.File, month *MonthRec, r *http.Request) {
    // Add entry from form
    recDate, err := time.Parse("2006-01-02", strings.TrimSpace(r.FormValue("date")))
    if err != nil {
      fmt.Println(err)
      recDate = time.Time{}
    }

    dayRecord := DayRec{
      Date: recDate,
      Category: r.FormValue("category"),
      Who: r.FormValue("who"),
      Currency: r.FormValue("currency"),
      Quantity: r.FormValue("quantity"),
      Comment: r.FormValue("comment"),
    }
    (*month).DayRecords = append((*month).DayRecords, dayRecord)

    sortRecordsByDate(month)

    tabName := "jan2021"

    // Add entry to actual file, at the bottom of the file
    rows, err := file.GetRows(tabName)
    if err != nil {
        fmt.Println(err)
    }

    lastRow := len(rows)
    // TODO row may brake columns to the right
    insertErr := file.InsertRow(tabName, lastRow)
    if insertErr != nil {
        fmt.Println(insertErr)
    }
    file.SetCellValue(tabName, fmt.Sprintf("A%d", lastRow), dayRecord.Date.Format("02/01/06"))
    file.SetCellValue(tabName, fmt.Sprintf("B%d", lastRow), dayRecord.Category)
    if dayRecord.Who == "A" {
      if dayRecord.Currency == "EUR" {
        file.SetCellValue(tabName, fmt.Sprintf("C%d", lastRow), dayRecord.Quantity)
      } else if dayRecord.Currency == "CHF" {
        file.SetCellValue(tabName, fmt.Sprintf("D%d", lastRow), dayRecord.Quantity)
      }
    } else if dayRecord.Who == "P" {
      if dayRecord.Currency == "EUR" {
        file.SetCellValue(tabName, fmt.Sprintf("E%d", lastRow), dayRecord.Quantity)
      } else if dayRecord.Currency == "CHF" {
        file.SetCellValue(tabName, fmt.Sprintf("F%d", lastRow), dayRecord.Quantity)
      }
    } else if dayRecord.Who == "B" {
      file.SetCellValue(tabName, fmt.Sprintf("G%d", lastRow), dayRecord.Quantity)
    }
    file.SetCellValue(tabName, fmt.Sprintf("H%d", lastRow), dayRecord.Comment)
}

// *******************************
// Sort records by ascending date within a month
// *******************************
func sortRecordsByDate(month *MonthRec) {
  sort.SliceStable((*month).DayRecords, func(i, j int) bool {
    return (*month).DayRecords[i].Date.Before((*month).DayRecords[j].Date)
  })
}

// *******************************
// Write changes done to the excel file
// *******************************
func writeExcel(file *excelize.File, month *MonthRec) http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {
    fmt.Println("Saving file")
    err := file.Save()
    if err != nil {
        fmt.Println(err)
    }

    tpl.Execute(w, *month)
  }
}


func readExcelFile(month *MonthRec, path string) (MonthRec, *excelize.File, error) {
  f, err := excelize.OpenFile(path)
  if err != nil {
    fmt.Println(err)
    return MonthRec{}, f, errors.New("Could not open file")
  }

  rows, err := f.GetRows("jan2021")
  if err != nil {
      fmt.Println(err)
      return MonthRec{}, f, errors.New("Error reading rows")
  }

  records := (*month).DayRecords

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
      dayRecord := DayRec {
        Date: entryDate,
        Category: row[1],
        Who: "A",
        Currency: "EUR",
        Quantity: row[2],
        Comment: row[7],
      }
      records = append(records, dayRecord);
    } else if row[3] != "" {
      dayRecord := DayRec {
        Date: entryDate,
        Category: row[1],
        Who: "A",
        Currency: "CHF",
        Quantity: row[3],
        Comment: row[7],
      }
      records = append(records, dayRecord);
    } else if row[4] != "" {
      dayRecord := DayRec {
        Date: entryDate,
        Category: row[1],
        Who: "P",
        Currency: "EUR",
        Quantity: row[4],
        Comment: row[7],
      }
      records = append(records, dayRecord);
    } else if row[5] != "" {
      dayRecord := DayRec {
        Date: entryDate,
        Category: row[1],
        Who: "P",
        Currency: "CHF",
        Quantity: row[5],
        Comment: row[7],
      }
      records = append(records, dayRecord);
    } else if row[6] != "" {
      dayRecord := DayRec {
        Date: entryDate,
        Category: row[1],
        Who: "B",
        Currency: "EUR",
        Quantity: row[6],
        Comment: row[7],
      }
      records = append(records, dayRecord);
    }
  }

  (*month).DayRecords = records

  // Get Max and Min
  (*month).MinDate = (*month).DayRecords[0].Date
  (*month).MaxDate = (*month).DayRecords[len((*month).DayRecords)-1].Date

  return *month, f, nil
}


func main() {
  port := "3000"

  fs := http.FileServer(http.Dir("assets"))

  // Read input file
  monthRecord := MonthRec{}
  file := &excelize.File{}
  if inputFileRead == false {
    filePath := os.Args[1]
    fmt.Println("Reading excel file: ", filePath)
    me, f, err := readExcelFile(&monthRecord, filePath)
    file = f
    monthRecord = me
    if err != nil {
      fmt.Println(err)
    }
    sortRecordsByDate(&monthRecord)
    inputFileRead = true
  }

  mux := http.NewServeMux()

  mux.Handle("/assets/", http.StripPrefix("/assets/", fs))
  mux.HandleFunc("/writeExcel", writeExcel(file, &monthRecord))
  mux.HandleFunc("/", indexHandler(file, &monthRecord))
  http.ListenAndServe(":"+port, mux)
}
