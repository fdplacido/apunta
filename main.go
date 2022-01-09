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

type PayerStats struct {
  Spent  float64
  Accum  float64
  Debt   float64
}

type MonthStats struct {
  AllPayersStats map[string]PayerStats
}

type MonthRec struct {
  MonthName      string
  MonthNum       int
  ActiveMonth    bool
  CurrStore      CurrencyEntry
  Stats          MonthStats
  DayRecords     []DayRec
}

type YearRec struct {
  excelFile       *excelize.File
  YearNum         int
  PrevYearDebt    map[string]float64
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
func (year *YearRec) indexHandler() http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {

    // Sort months by number
    year.sortMonthsByDate()

    // Make monthly statistics if there are none
    for index, month := range year.MonthRecords {
      // TODO Introduce checks to not calculate this every time
      prevMonthNum := month.MonthNum - 1
      if prevMonthNum < 1 {
        year.MonthRecords[index].Stats = month.calcStats(nil)
      } else {
        year.MonthRecords[index].Stats = month.calcStats(&(year.MonthRecords[index - 1]))
      }
    }

    tpl.Execute(w, year)
  }
}


// *******************************
// Calculate statistics for this month
// *******************************
func (month *MonthRec) calcStats(prevMonth *MonthRec) MonthStats {

  if month.Stats.AllPayersStats == nil {
    month.Stats.AllPayersStats = map[string]PayerStats{}
  } else {
    // Reset stats if recalculating the whole month
    month.Stats.AllPayersStats = map[string]PayerStats{}
  }

  // Get previous month debts, add them to Accumulated for this month
  if prevMonth != nil && month.MonthNum > 1 {
    for key, value := range month.Stats.AllPayersStats {
      if prevStats, prevOk := prevMonth.Stats.AllPayersStats[key]; prevOk {
        value.Accum += prevStats.Debt
        month.Stats.AllPayersStats[key] = PayerStats{value.Spent, value.Accum, value.Debt}
      }
    }
  }

  // Calculate spent
  allSpent := make([]float64, 0)
  for _, dayRec := range month.DayRecords {
    // Store special case for all to process at the end
    // Skip special case statistics
    if dayRec.Who == "B" || dayRec.Who == "All" {
      if convQuantity, err := strconv.ParseFloat(dayRec.Quantity, 64); err == nil {
        allSpent = append(allSpent, convQuantity)
      }
      continue
    }

    if stats, ok := month.Stats.AllPayersStats[dayRec.Who]; ok {
      // Key already thare, add
      if convQuantity, err := strconv.ParseFloat(dayRec.Quantity, 64); err == nil {
        stats.Spent += convQuantity
        month.Stats.AllPayersStats[dayRec.Who] = stats
      }
    } else {
      // no key, just create the value
      if convQuantity, err := strconv.ParseFloat(dayRec.Quantity, 64); err == nil {
        stats = PayerStats{convQuantity, 0.0, 0.0}
        month.Stats.AllPayersStats[dayRec.Who] = stats
      }
    }
  }

  // Divide "All" costs between all payers
  numPayers := float64(len(month.Stats.AllPayersStats))
  for _, entrySpent := range allSpent {
    for key, value := range month.Stats.AllPayersStats {
      value.Spent += entrySpent/numPayers
      month.Stats.AllPayersStats[key] = PayerStats{value.Spent, value.Accum, value.Debt}
    }
  }

  // Calculate Accumulated for all payers
  for key, value := range month.Stats.AllPayersStats {
    value.Accum += value.Spent
    month.Stats.AllPayersStats[key] = PayerStats{value.Spent, value.Accum, value.Debt}
  }

  // Calculate Debt between all payers
  // Find top payer and equal all other payers to pay as much as the top
  totalAccum := 0.0
  topAmount := 0.0
  for _, value := range month.Stats.AllPayersStats {
    // Save top payer
    if value.Accum > topAmount {
      topAmount = value.Accum
    }

    totalAccum += value.Accum
  }

  amountAllPay := totalAccum / float64(len(month.Stats.AllPayersStats) - 1)
  for key, value := range month.Stats.AllPayersStats {
    if value.Accum >= topAmount {
      value.Debt = 0.0
    } else {
      value.Debt = topAmount - value.Accum
    }
    month.Stats.AllPayersStats[key] = PayerStats{value.Spent, value.Accum, value.Debt}
  }

  return month.Stats
}

// *******************************
// Add Entry to the Excel file
// TODO do this only when writting the actual excel file
// TODO there needs to be a stack of operations to do when saving
// *******************************
func (year *YearRec) addEntryToExcel(ptrMonthRec *MonthRec, dayRecord *DayRec) {
  // Add entry to actual file, at the bottom of the file
  rows, err := year.excelFile.GetRows((*ptrMonthRec).MonthName)
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
// Create an empty MonthRec
// *******************************
func newMonthRec() *MonthRec {
  monthRecord := &MonthRec{}
  monthRecord.ActiveMonth = true
  monthRecord.Stats.AllPayersStats = map[string]PayerStats{}
  return monthRecord
}



// *******************************
// Create an empty YearRec
// *******************************
func newYearRec() *YearRec {
  yearRecord := &YearRec{}
  yearRecord.Payers = append(yearRecord.Payers, "All")
  return yearRecord
}

// *******************************
// Add new category to the list
// *******************************
func (year *YearRec) addCategory() http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {
    newCategory := strings.TrimSpace(r.FormValue("newCategory"))
    year.Categories = append(year.Categories, newCategory)

    tpl.Execute(w, year)
  }
}


// *******************************
// Add new payer to the list
// *******************************
func (year *YearRec) addPayer() http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {
    newPayer := strings.TrimSpace(r.FormValue("newPayer"))
    year.Payers = append(year.Payers, newPayer)

    tpl.Execute(w, year)
  }
}

// *******************************
// Add new currency to the list
// *******************************
func (year *YearRec) addCurrency() http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {
    newCurrency := strings.TrimSpace(r.FormValue("newCurrency"))
    year.Currencies = append(year.Currencies, newCurrency)

    tpl.Execute(w, year)
  }
}


// *******************************
// Add previous year debt data
// *******************************
func (year *YearRec) addPreviousYearDebts() http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {

    prevYearName := strings.TrimSpace(r.FormValue("prevYearDebtName"))
    prevYearAmount := strings.TrimSpace(r.FormValue("prevYearDebtAmount"))

    if year.PrevYearDebt == nil {
      year.PrevYearDebt = map[string]float64{}
    }

    if convQuantity, err := strconv.ParseFloat(prevYearAmount, 64); err == nil {
      year.PrevYearDebt[prevYearName] = convQuantity
    }

    tpl.Execute(w, year)
  }
}


// *******************************
// Add new entry
// *******************************
func (year *YearRec) addEntry() http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {
    // Add entry from form
    recDate, err := time.Parse("2006-01-02", strings.TrimSpace(r.FormValue("date")))
    if err != nil {
      fmt.Println(err)
      recDate = time.Time{}
    }

    // Check correct year
    if recDate.Year() != year.YearNum {
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
    for index, month := range year.MonthRecords {
      if (int(recDate.Month())) == month.MonthNum {

        fmt.Println("Average exch rate for this month: ", year.MonthRecords[index].CurrStore.AvgVal)
        // Check if there is an exchange rate for this month
        if year.MonthRecords[index].CurrStore.AvgVal == 0.0 {
          // Get exchange rate from openexchangerates
          rate, err := exchRages.GetRate("CHF", "EUR")
          if err != nil {
            fmt.Println(err)
          }
          year.MonthRecords[index].CurrStore.AvgVal = rate
          fmt.Println("Got exchange rate: ", year.MonthRecords[index].CurrStore.AvgVal)
        }

        year.MonthRecords[index].DayRecords = append(year.MonthRecords[index].DayRecords, dayRecord)
        year.MonthRecords[index].sortRecordsByDate()
        break
      }
    }

    // Recalculate month statistics
    for index, month := range year.MonthRecords {
      // TODO Introduce checks to not calculate this every time
      // TODO calculate only from current month, without the previous ones
      prevMonthNum := month.MonthNum - 1
      if prevMonthNum < 1 {
        year.MonthRecords[index].Stats = month.calcStats(nil)
      } else {
        year.MonthRecords[index].Stats = month.calcStats(&(year.MonthRecords[index - 1]))
      }
    }

    tpl.Execute(w, year)
  }
}


// *******************************
// Change active month to selected month
// *******************************
func (year *YearRec) changeToSheet() http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {

    r.ParseForm()
    selectedSheet := r.FormValue("changeSheet")

    for index, month := range year.MonthRecords {
      if selectedSheet != month.MonthName {
        month.ActiveMonth = false
        year.MonthRecords[index] = month
      } else {
        month.ActiveMonth = true
        year.MonthRecords[index] = month
      }
    }

    tpl.Execute(w, year)
  }
}


// *******************************
// Sort records by ascending date within a month
// *******************************
func (month *MonthRec) sortRecordsByDate() {
  sort.SliceStable(month.DayRecords, func(i, j int) bool {
    return month.DayRecords[i].Date.Before(month.DayRecords[j].Date)
  })
}

// *******************************
// Sort months by ascending date within a year
// *******************************
func (year *YearRec) sortMonthsByDate() {
  sort.SliceStable(year.MonthRecords, func(i, j int) bool {
    return year.MonthRecords[i].MonthNum < year.MonthRecords[j].MonthNum
  })
}

// *******************************
// Write changes done to the excel file
// *******************************
func (year *YearRec) writeExcel() http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {
    fmt.Println("Saving XLSX file...")
    err := year.excelFile.Save()
    if err != nil {
        fmt.Println(err)
    }

    tpl.Execute(w, year)
  }
}


// *******************************
// Write changes into a JSON file
// *******************************
func (year *YearRec) writeJson(fileName string) http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {
    b, err := json.MarshalIndent(year, "", " ")
    if err != nil {
        fmt.Println(err)
        return
    }
    _ = ioutil.WriteFile(fileName, b, 0644)

    tpl.Execute(w, year)
  }
}


// *******************************
// Add sheet given a name
// *******************************
func (year *YearRec) addSheet() http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {

    monthRec := newMonthRec()
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
    year.YearNum = sheetYear

    year.MonthRecords = append(year.MonthRecords, *monthRec)

    tpl.Execute(w, year)
  }
}


func (year *YearRec) readExcelFile(path string) error {
  file, err := excelize.OpenFile(path)
  if err != nil {
    fmt.Println(err)
    return errors.New("Could not open file "+path)
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
      return errors.New("Error reading rows for sheet "+name)
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

    year.MonthRecords = append(year.MonthRecords, monthRec)

    fmt.Print(".")
  }

  fmt.Println(" File read!")

  return nil
}


func main() {
  port := "3000"

  yearRecord := newYearRec()

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
      err := yearRecord.readExcelFile(filePath)
      if err != nil {
        fmt.Println(err)
      }
      for _, month := range yearRecord.MonthRecords {
        month.sortRecordsByDate()
      }
      inputFileRead = true

      yearRecord.writeJson("test.json")
    }
  } else if len(os.Args) == 1 {
    fmt.Println("No input file: creating empty record")
  }

  fmt.Println("Listening on localhost:"+port)

  // Serve assets folder
  fs := http.FileServer(http.Dir("assets"))

  mux := http.NewServeMux()

  mux.Handle("/assets/", http.StripPrefix("/assets/", fs))

  mux.HandleFunc("/writeExcel", yearRecord.writeExcel())
  mux.HandleFunc("/writeJSON", yearRecord.writeJson("test_1.json"))

  mux.HandleFunc("/addCategory", yearRecord.addCategory())
  mux.HandleFunc("/addWho", yearRecord.addPayer())
  mux.HandleFunc("/addCurrency", yearRecord.addCurrency())

  mux.HandleFunc("/changeSheet", yearRecord.changeToSheet())

  mux.HandleFunc("/addSheet", yearRecord.addSheet())
  mux.HandleFunc("/addEntry", yearRecord.addEntry())
  mux.HandleFunc("/", yearRecord.indexHandler())

  http.ListenAndServe(":"+port, mux)

  fmt.Println("Listening on localhost:"+port)
}
