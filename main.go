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
  MaxDate     time.Time
  MinDate     time.Time
  MonthName   string
  ActiveMonth bool
  DayRecords  []DayRec
}

type YearRec struct {
  excelFile       *excelize.File
  MonthRecords    []MonthRec
}

var tpl = template.Must(template.ParseFiles("index.html"))
var inputFileRead = false

func indexHandler(year *YearRec) http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
      tpl.Execute(w, *year)
      return
    }

    addNewEntry(year, r, "dec2021")

    tpl.Execute(w, *year)
  }
}

func addNewEntry(year *YearRec, r *http.Request, sheetName string) {
    // Add entry from form
    recDate, err := time.Parse("2006-01-02", strings.TrimSpace(r.FormValue("date")))
    if err != nil {
      fmt.Println(err)
      recDate = time.Time{}
    }

    // TODO change insertion to happen to the correct tab
    var ptrMonthRec *MonthRec
    for _, month := range (*year).MonthRecords {
      if sheetName == month.MonthName {
        ptrMonthRec = &month
      }
    }

    dayRecord := DayRec{
      Date: recDate,
      Category: r.FormValue("category"),
      Who: r.FormValue("who"),
      Currency: r.FormValue("currency"),
      Quantity: r.FormValue("quantity"),
      Comment: r.FormValue("comment"),
    }
    (*ptrMonthRec).DayRecords = append((*ptrMonthRec).DayRecords, dayRecord)

    sortRecordsByDate(ptrMonthRec)

    // Add entry to actual file, at the bottom of the file
    rows, err := year.excelFile.GetRows(sheetName)
    if err != nil {
        fmt.Println(err)
    }

    lastRow := len(rows)
    // TODO row may brake columns to the right
    insertErr := year.excelFile.InsertRow(sheetName, lastRow)
    if insertErr != nil {
        fmt.Println(insertErr)
    }
    year.excelFile.SetCellValue(sheetName, fmt.Sprintf("A%d", lastRow), dayRecord.Date.Format("02/01/06"))
    year.excelFile.SetCellValue(sheetName, fmt.Sprintf("B%d", lastRow), dayRecord.Category)
    if dayRecord.Who == "A" {
      if dayRecord.Currency == "EUR" {
        year.excelFile.SetCellValue(sheetName, fmt.Sprintf("C%d", lastRow), dayRecord.Quantity)
      } else if dayRecord.Currency == "CHF" {
        year.excelFile.SetCellValue(sheetName, fmt.Sprintf("D%d", lastRow), dayRecord.Quantity)
      }
    } else if dayRecord.Who == "P" {
      if dayRecord.Currency == "EUR" {
        year.excelFile.SetCellValue(sheetName, fmt.Sprintf("E%d", lastRow), dayRecord.Quantity)
      } else if dayRecord.Currency == "CHF" {
        year.excelFile.SetCellValue(sheetName, fmt.Sprintf("F%d", lastRow), dayRecord.Quantity)
      }
    } else if dayRecord.Who == "B" {
      year.excelFile.SetCellValue(sheetName, fmt.Sprintf("G%d", lastRow), dayRecord.Quantity)
    }
    year.excelFile.SetCellValue(sheetName, fmt.Sprintf("H%d", lastRow), dayRecord.Comment)
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
func writeExcel(year *YearRec) http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {
    fmt.Println("Saving file...")
    err := year.excelFile.Save()
    if err != nil {
        fmt.Println(err)
    }

    tpl.Execute(w, *year)
  }
}


func readExcelFile(year *YearRec, path string) (YearRec, error) {
  file, err := excelize.OpenFile(path)
  if err != nil {
    fmt.Println(err)
    return YearRec{}, errors.New("Could not open file "+path)
  }
  year.excelFile = file

  // Get ordered set of sheet indexes
  var sheetIndexes []int
  for index, _ := range year.excelFile.GetSheetMap() {
    sheetIndexes = append(sheetIndexes, index)
  }
  sort.Ints(sheetIndexes)

  activeMonth := year.excelFile.GetActiveSheetIndex()

  // Fill structs with info from excel sheets, sorted
  for index := range sheetIndexes {
    name := year.excelFile.GetSheetName(index)

    // Fill a new month Record
    monthRec := MonthRec{
      ActiveMonth: false,
    }
    monthRec.MonthName = name

    // Set active month
    if activeMonth == index {
      monthRec.ActiveMonth = true
    }

    rows, err := year.excelFile.GetRows(name)
    if err != nil {
      fmt.Println(err)
      return YearRec{}, errors.New("Error reading rows for sheet "+name)
    }

    dayRecords := monthRec.DayRecords

    // Loop over all records for this month
    for index, row := range rows {
      // Skip first row
      // TODO check if header is correct for robustness
      if index == 0 {
        continue
      }
      // Only process if there is something in the row
      if len(row) == 0 {
        continue
      }

      // Skip row if there is no entry
      if row[0] == "" {
        continue
      }

      // Parse Date
      const layout = "02/01/06"
      entryDate, err := time.Parse(layout, strings.TrimSpace(row[0]))
      if err != nil {
        fmt.Println(row[0])
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
        dayRecords = append(dayRecords, dayRecord);
      } else if row[3] != "" {
        dayRecord := DayRec {
          Date: entryDate,
          Category: row[1],
          Who: "A",
          Currency: "CHF",
          Quantity: row[3],
          Comment: row[7],
        }
        dayRecords = append(dayRecords, dayRecord);
      } else if row[4] != "" {
        dayRecord := DayRec {
          Date: entryDate,
          Category: row[1],
          Who: "P",
          Currency: "EUR",
          Quantity: row[4],
          Comment: row[7],
        }
        dayRecords = append(dayRecords, dayRecord);
      } else if row[5] != "" {
        dayRecord := DayRec {
          Date: entryDate,
          Category: row[1],
          Who: "P",
          Currency: "CHF",
          Quantity: row[5],
          Comment: row[7],
        }
        dayRecords = append(dayRecords, dayRecord);
      } else if row[6] != "" {
        dayRecord := DayRec {
          Date: entryDate,
          Category: row[1],
          Who: "B",
          Currency: "EUR",
          Quantity: row[6],
          Comment: row[7],
        }
        dayRecords = append(dayRecords, dayRecord);
      }
    }

    monthRec.DayRecords = dayRecords

    // Get Max and Min
    monthRec.MinDate = monthRec.DayRecords[0].Date
    monthRec.MaxDate = monthRec.DayRecords[len(monthRec.DayRecords)-1].Date

    (*year).MonthRecords = append((*year).MonthRecords, monthRec)
  }

  return *year, nil
}


func main() {
  port := "3000"

  fs := http.FileServer(http.Dir("assets"))

  // Read input file
  yearRecord := YearRec{}
  // file := &excelize.File{}
  if inputFileRead == false {
    filePath := os.Args[1]
    fmt.Println("Reading input file: ", filePath)
    yr, err := readExcelFile(&yearRecord, filePath)
    yearRecord = yr
    if err != nil {
      fmt.Println(err)
    }
    for _, month := range yearRecord.MonthRecords {
      sortRecordsByDate(&month)
    }
    inputFileRead = true
  }

  mux := http.NewServeMux()

  mux.Handle("/assets/", http.StripPrefix("/assets/", fs))
  mux.HandleFunc("/writeExcel", writeExcel(&yearRecord))
  mux.HandleFunc("/", indexHandler(&yearRecord))
  http.ListenAndServe(":"+port, mux)

  fmt.Println("Listening on localhost:"+port)
}
