package main

import (
  "fmt"
  "net/http"
  "html/template"
  "errors"
  "time"
  "strings"
  "strconv"
  "sort"
  "path/filepath"
  "os"
  "encoding/json"
  "io/ioutil"

  "apunta/exchRates"

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

type CurrencyEntry struct {
  CurrFrom  string
  CurrTo    string
  AvgVal    float64
}

type MonthRec struct {
  MonthName   string
  MonthNum    int
  ActiveMonth bool
  CurrStore   CurrencyEntry
  DayRecords  []DayRec
}

type YearRec struct {
  excelFile       *excelize.File
  YearNum         int
  Categories      []string
  Payers          []string
  Currencies      []string
  MonthRecords    []MonthRec
}

var tpl = template.Must(template.ParseFiles("index.html"))
var inputFileRead = false


// *******************************
// Entry point from loaded or empty entries
// *******************************
func indexHandler(year *YearRec) http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {
    tpl.Execute(w, *year)
  }
}

// *******************************
// Add Entry to the Excel file
// TODO do this only when writting the actual excel file
// TODO there needs to be a stack of operations to do when saving
// *******************************
func addEntryToExcel(year *YearRec, ptrMonthRec *MonthRec, dayRecord *DayRec) {
  // Add entry to actual file, at the bottom of the file
  rows, err := (*year).excelFile.GetRows((*ptrMonthRec).MonthName)
  if err != nil {
      fmt.Println(err)
  }

  lastRow := len(rows)
  // TODO row may brake columns to the right
  insertErr := year.excelFile.InsertRow((*ptrMonthRec).MonthName, lastRow)
  if insertErr != nil {
      fmt.Println(insertErr)
  }
  year.excelFile.SetCellValue((*ptrMonthRec).MonthName, fmt.Sprintf("A%d", lastRow), (*dayRecord).Date.Format("02/01/06"))
  year.excelFile.SetCellValue((*ptrMonthRec).MonthName, fmt.Sprintf("B%d", lastRow), (*dayRecord).Category)
  if (*dayRecord).Who == "A" {
    if (*dayRecord).Currency == "EUR" {
      year.excelFile.SetCellValue((*ptrMonthRec).MonthName, fmt.Sprintf("C%d", lastRow), (*dayRecord).Quantity)
    } else if (*dayRecord).Currency == "CHF" {
      year.excelFile.SetCellValue((*ptrMonthRec).MonthName, fmt.Sprintf("D%d", lastRow), (*dayRecord).Quantity)
    }
  } else if (*dayRecord).Who == "P" {
    if (*dayRecord).Currency == "EUR" {
      year.excelFile.SetCellValue((*ptrMonthRec).MonthName, fmt.Sprintf("E%d", lastRow), (*dayRecord).Quantity)
    } else if (*dayRecord).Currency == "CHF" {
      year.excelFile.SetCellValue((*ptrMonthRec).MonthName, fmt.Sprintf("F%d", lastRow), (*dayRecord).Quantity)
    }
  } else if (*dayRecord).Who == "B" {
    year.excelFile.SetCellValue((*ptrMonthRec).MonthName, fmt.Sprintf("G%d", lastRow), (*dayRecord).Quantity)
  }
  year.excelFile.SetCellValue((*ptrMonthRec).MonthName, fmt.Sprintf("H%d", lastRow), (*dayRecord).Comment)

  fmt.Println("Inserted: ", (*dayRecord))
}



// *******************************
// Add new category to the list
// *******************************
func addCategory(year *YearRec) http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {
    newCategory := strings.TrimSpace(r.FormValue("newCategory"))
    (*year).Categories = append((*year).Categories, newCategory)

    tpl.Execute(w, *year)
  }
}


// *******************************
// Add new payer to the list
// *******************************
func addPayer(year *YearRec) http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {
    newPayer := strings.TrimSpace(r.FormValue("newPayer"))
    (*year).Payers = append((*year).Payers, newPayer)

    tpl.Execute(w, *year)
  }
}

// *******************************
// Add new currency to the list
// *******************************
func addCurrency(year *YearRec) http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {
    newCurrency := strings.TrimSpace(r.FormValue("newCurrency"))
    (*year).Currencies = append((*year).Currencies, newCurrency)

    tpl.Execute(w, *year)
  }
}


// *******************************
// Add new entry
// *******************************
func addEntry(year *YearRec) http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {
    // Add entry from form
    recDate, err := time.Parse("2006-01-02", strings.TrimSpace(r.FormValue("date")))
    if err != nil {
      fmt.Println(err)
      recDate = time.Time{}
    }

    // Check correct year
    if recDate.Year() != (*year).YearNum {
      fmt.Println("Chosen date belongs to a different year")
      return
    }

    r.ParseForm()
    dayRecord := DayRec{
      Date: recDate,
      Category: r.FormValue("category"),
      Who: r.FormValue("who"),
      Currency: r.FormValue("currency"),
      Quantity: r.FormValue("quantity"),
      Comment: r.FormValue("comment"),
    }

    // Find correct month to insert to
    for index, month := range (*year).MonthRecords {
      if (int(recDate.Month())) == month.MonthNum {

        fmt.Println("Average exch rate for this month: ", (*year).MonthRecords[index].CurrStore.AvgVal)
        // Check if there is an exchange rate for this month
        if (*year).MonthRecords[index].CurrStore.AvgVal == 0.0 {
          // Get exchange rate from openexchangerates
          (*year).MonthRecords[index].CurrStore.AvgVal = exchRages.GetRate("CHF", "EUR")
          fmt.Println("Got exchange rate: ", (*year).MonthRecords[index].CurrStore.AvgVal)
        }

        (*year).MonthRecords[index].DayRecords = append((*year).MonthRecords[index].DayRecords, dayRecord)
        sortRecordsByDate(&(*year).MonthRecords[index])
        break
      }
    }

    tpl.Execute(w, *year)
  }
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
    fmt.Println("Saving XLSX file...")
    err := year.excelFile.Save()
    if err != nil {
        fmt.Println(err)
    }

    tpl.Execute(w, *year)
  }
}


// *******************************
// Write changes into a JSON file
// *******************************
func writeJson(year *YearRec, fileName string) http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {
    b, err := json.MarshalIndent((*year), "", " ")
    if err != nil {
        fmt.Println(err)
        return
    }
    _ = ioutil.WriteFile(fileName, b, 0644)

    tpl.Execute(w, *year)
  }
}


// *******************************
// Add sheet given a name
// *******************************
func addSheet(year *YearRec) http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {

    monthRec := MonthRec{
      ActiveMonth: true,
    }
    monthRec.MonthName = strings.TrimSpace(r.FormValue("sheetName"))
    monthNum, err := strconv.Atoi(strings.TrimSpace(r.FormValue("monthNumber")))
    if err != nil {
      fmt.Println(err)
      return
    }
    monthRec.MonthNum = monthNum

    sheetYear, err := strconv.Atoi(strings.TrimSpace(r.FormValue("sheetYear")))
    if err != nil {
      fmt.Println("Wrong format entered for year, use YYYY")
      fmt.Println(err)
      return
    }
    (*year).YearNum = sheetYear

    (*year).MonthRecords = append((*year).MonthRecords, monthRec)

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

    (*year).MonthRecords = append((*year).MonthRecords, monthRec)

    fmt.Print(".")
  }

  fmt.Println(" File read!")

  return *year, nil
}


func main() {
  port := "3000"

  yearRecord := YearRec{}

  // Check input file type
  if inputFileRead == false && len(os.Args) == 2 {
    filePath := os.Args[1]
    extensionType := filepath.Ext(filePath)
    if extensionType == ".json" {
      fmt.Println("Reading input file: filePath")
      jsonFile, err := os.Open(filePath)
      if err != nil {
          fmt.Println(err)
      }
      defer jsonFile.Close()
      byteValue, _ := ioutil.ReadAll(jsonFile)
      json.Unmarshal(byteValue, &yearRecord)

    } else if extensionType == ".xlsx" {
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

      writeJson(&yearRecord, "test.json")
    }
  } else if len(os.Args) == 1 {
    fmt.Println("No input file: creating empty record")
  }

  fmt.Println("Listening on localhost:"+port)

  // Serve assets folder
  fs := http.FileServer(http.Dir("assets"))

  mux := http.NewServeMux()

  mux.Handle("/assets/", http.StripPrefix("/assets/", fs))
  mux.HandleFunc("/writeExcel", writeExcel(&yearRecord))
  mux.HandleFunc("/writeJSON", writeJson(&yearRecord, "test.json"))

  mux.HandleFunc("/addCategory", addCategory(&yearRecord))
  mux.HandleFunc("/addWho", addPayer(&yearRecord))
  mux.HandleFunc("/addCurrency", addCurrency(&yearRecord))

  mux.HandleFunc("/addSheet", addSheet(&yearRecord))
  mux.HandleFunc("/addEntry", addEntry(&yearRecord))
  mux.HandleFunc("/", indexHandler(&yearRecord))
  http.ListenAndServe(":"+port, mux)

  fmt.Println("Listening on localhost:"+port)
}
