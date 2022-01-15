package main

import (
  "fmt"
  "net/http"
  "html/template"
  "time"
  "strings"
  "strconv"
  "sort"
  "path/filepath"
  "os"
  "encoding/json"
  "io/ioutil"

  "apunta/exchRates"
)

type EntryRec struct {
  Date       time.Time
  Category   string
  PersonName string
  Currency   string
  Amount     float64
  Comment    string
}

type ExRateEntry struct {
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
  StartDate    time.Time
  GroupName    string
  ActiveGroup  bool
  AvgExch      ExRateEntry
  Stats        MonthStats
  EntryRecords []EntryRec
}

type Document struct {
  PrevDebt     map[string]float64
  Categories   []string
  Payers       []string
  Currencies   []string
  MonthRecs    []MonthRec
}

// Global stuff
var tpl = template.Must(template.ParseFiles("index.html"))
var inputFileRead = false


// *******************************
// Entry point from loaded or empty entries
// *******************************
func (doc *Document) indexHandler() http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {

    // TODO this should't be called every time, they should always be sorted
    doc.sortMonthsByDate()

    // TODO don't recalculate stats on just opening a file
    doc.calcAllStats()

    tpl.Execute(w, doc)
  }
}

// *******************************
// Calculate statistics for this month
// *******************************
func (month *MonthRec) calcStats(prevMonth *MonthRec, prevDebtData map[string]float64) MonthStats {

  // Reset stats if recalculating the whole month / init map
  month.Stats.AllPayersStats = map[string]PayerStats{}

  // Include previous file debt data for first month
  if prevMonth == nil {
    for name, debtValue := range prevDebtData {
      if stats, ok := month.Stats.AllPayersStats[name]; ok {
        stats.Accum += (-1 * debtValue)
        month.Stats.AllPayersStats[name] = PayerStats{stats.Spent, stats.Accum, stats.Debt}
      } else {
        month.Stats.AllPayersStats[name] = PayerStats{0.0, debtValue * -1.0, 0.0}
      }
    }
  } else { // Get previous month debts, add them to Accumulated for this month
    for key, value := range month.Stats.AllPayersStats {
      if prevStats, prevOk := prevMonth.Stats.AllPayersStats[key]; prevOk {
        value.Accum += (-1 * prevStats.Debt)
        month.Stats.AllPayersStats[key] = PayerStats{value.Spent, value.Accum, value.Debt}
      } else {
        month.Stats.AllPayersStats[key] = PayerStats{value.Spent, (-1 * prevStats.Debt), value.Debt}
      }
    }
  }

  // Calculate spent
  allSpent := make([]float64, 0)
  for _, dayRec := range month.EntryRecords {
    // Store special case for all to process at the end
    // Skip special case statistics
    if dayRec.PersonName == "B" || dayRec.PersonName == "All" {
      allSpent = append(allSpent, dayRec.Amount)
      continue
    }

    if stats, ok := month.Stats.AllPayersStats[dayRec.PersonName]; ok {
      // Key already thare, add
      stats.Spent += dayRec.Amount
      month.Stats.AllPayersStats[dayRec.PersonName] = stats
    } else {
      // no key, just create the value
      stats = PayerStats{dayRec.Amount, 0.0, 0.0}
      month.Stats.AllPayersStats[dayRec.PersonName] = stats
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
// Create an empty MonthRec
// *******************************
func newMonthRec() *MonthRec {
  monthRecord := &MonthRec{}
  monthRecord.ActiveGroup = false
  monthRecord.Stats.AllPayersStats = map[string]PayerStats{}
  return monthRecord
}


// *******************************
// Create an empty Document
// *******************************
func newDocument() *Document {
  doc := &Document{}
  // Always have the "All" payers
  doc.Payers = append(doc.Payers, "All")
  return doc
}


// *******************************
// Add new category to the list
// *******************************
func (doc *Document) addCategory() http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {
    newCategory := strings.TrimSpace(r.FormValue("newCategory"))
    doc.Categories = append(doc.Categories, newCategory)

    tpl.Execute(w, doc)
  }
}


// *******************************
// Prepend string with fewer allocations,
// compared to using compose literal append([]string{1}, x...)
// *******************************
func prependStr(x []string, y string) []string {
    x = append(x, "")
    copy(x[1:], x)
    x[0] = y
    return x
}


// *******************************
// Add new payer to the list
// *******************************
func (doc *Document) addPayer() http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {
    newPayer := strings.TrimSpace(r.FormValue("newPayer"))
    // Put new payer on top
    doc.Payers = prependStr(doc.Payers, newPayer)

    tpl.Execute(w, doc)
  }
}

// *******************************
// Add new currency to the list
// *******************************
func (doc *Document) addCurrency() http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {
    newCurrency := strings.TrimSpace(r.FormValue("newCurrency"))
    doc.Currencies = append(doc.Currencies, newCurrency)

    tpl.Execute(w, doc)
  }
}


// *******************************
// Calculate all months statistics
// *******************************
func (doc *Document) calcAllStats() {
  // Recalculate month statistics
  // Months sorted by date is assumed
  for index, month := range doc.MonthRecs {
    // TODO Introduce checks to not calculate this every time
    // TODO calculate only from current month, without the previous ones
    if index == 0 {
      fmt.Println("calculating first month")
      doc.MonthRecs[index].Stats = month.calcStats(nil, doc.PrevDebt)
    } else {
      fmt.Println("calculating other month")
      // TODO probably doesn't need a pointer to all the data
      doc.MonthRecs[index].Stats = month.calcStats(&(doc.MonthRecs[index - 1]), doc.PrevDebt)
    }
  }
}


// *******************************
// Add previous document debt data
// This is a manual step to be introduced by the user
// *******************************
func (doc *Document) addPreviousDebts() http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {

    prevName := strings.TrimSpace(r.FormValue("prevDebtName"))
    prevAmount := strings.TrimSpace(r.FormValue("prevDebtAmount"))

    if doc.PrevDebt == nil {
      doc.PrevDebt = map[string]float64{}
    }

    if convQuantity, err := strconv.ParseFloat(prevAmount, 64); err == nil {
      doc.PrevDebt[prevName] = convQuantity
    }

    doc.calcAllStats()

    tpl.Execute(w, doc)
  }
}


// *******************************
// Helper function to check if month and year are the same for two dates
// *******************************
func isSameMonthYear(a, b time.Time) bool {
  aM := int(a.Month())
  bM := int(b.Month())

  if (aM == bM) && (a.Year() == b.Year()) {
    return true
  } else {
    return false
  }
}


// *******************************
// Add new entry
// *******************************
func (doc *Document) addEntry() http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {
    // Add entry from form
    recDate, err := time.Parse("2006-01-02", strings.TrimSpace(r.FormValue("date")))
    if err != nil {
      fmt.Println(err)
      recDate = time.Time{}
    }

    r.ParseForm()

    entry := EntryRec{
      Date: recDate,
      Category: r.FormValue("category"),
      PersonName: r.FormValue("who"),
      Currency: r.FormValue("currency"),
      Amount: 0.0,
      Comment: r.FormValue("comment"),
    }

    if convAmount, err := strconv.ParseFloat(r.FormValue("quantity"), 64); err == nil {
      entry.Amount = convAmount
    } else {
      fmt.Println("There was an error processing the quantity input: not a float64")
    }

    // Find correct month to insert to
    for index, month := range doc.MonthRecs {
      if isSameMonthYear(recDate, month.StartDate) {

        fmt.Println("Average exch rate for this month: ", doc.MonthRecs[index].AvgExch.AvgVal)
        // Check if there is an exchange rate for this month
        if doc.MonthRecs[index].AvgExch.AvgVal == 0.0 {
          // Get exchange rate from openexchangerates
          rate, err := exchRages.GetRate("CHF", "EUR")
          if err != nil {
            fmt.Println(err)
          }
          doc.MonthRecs[index].AvgExch.AvgVal = rate
          fmt.Println("Got exchange rate: ", doc.MonthRecs[index].AvgExch.AvgVal)
        }

        doc.MonthRecs[index].EntryRecords = append(doc.MonthRecs[index].EntryRecords, entry)
        doc.MonthRecs[index].sortRecordsByDate()
        break
      }
    }

    doc.calcAllStats()

    tpl.Execute(w, doc)
  }
}


// *******************************
// Change active month to selected month
// *******************************
func (doc *Document) changeToSheet() http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {

    r.ParseForm()
    selectedSheet := r.FormValue("changeSheet")

    for index, month := range doc.MonthRecs {
      if selectedSheet != month.GroupName {
        month.ActiveGroup = false
        doc.MonthRecs[index] = month
      } else {
        month.ActiveGroup = true
        doc.MonthRecs[index] = month
      }
    }

    tpl.Execute(w, doc)
  }
}


// *******************************
// Sort records by ascending date within a month
// *******************************
func (month *MonthRec) sortRecordsByDate() {
  sort.SliceStable(month.EntryRecords, func(i, j int) bool {
    return month.EntryRecords[i].Date.Before(month.EntryRecords[j].Date)
  })
}

// *******************************
// Sort months by ascending date within a document
// *******************************
func (doc *Document) sortMonthsByDate() {
  sort.SliceStable(doc.MonthRecs, func(i, j int) bool {
    return doc.MonthRecs[i].StartDate.Before(doc.MonthRecs[j].StartDate)
  })
}


// *******************************
// Write changes into a JSON file
// *******************************
func (doc *Document) writeJson(fileName string) http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {
    b, err := json.MarshalIndent(doc, "", " ")
    if err != nil {
        fmt.Println(err)
        return
    }
    _ = ioutil.WriteFile(fileName, b, 0644)

    tpl.Execute(w, doc)
  }
}


// *******************************
// Add sheet given a name
// *******************************
func (doc *Document) addSheet() http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {

    monthRec := newMonthRec()
    // Mark new month as active
    for index, _ := range doc.MonthRecs {
      doc.MonthRecs[index].ActiveGroup = false
    }
    monthRec.ActiveGroup = true
    monthRec.GroupName = strings.TrimSpace(r.FormValue("sheetName"))

    if monthRec.GroupName == "" {
      monthRec.GroupName = r.FormValue("monthYearSheet")
    }

    monthYearSlice := strings.Split(r.FormValue("monthYearSheet"), "-")

    sheetYear, err := strconv.Atoi(monthYearSlice[0])
    if err != nil {
      fmt.Println(err)
      return
    }

    monthNum, err := strconv.Atoi(monthYearSlice[1])
    if err != nil {
      fmt.Println(err)
      return
    }

    firstDayMonth := time.Date(sheetYear, time.Month(monthNum), 1, 0, 0, 0, 0, time.Now().Location())
    monthRec.StartDate = firstDayMonth

    doc.MonthRecs = append(doc.MonthRecs, *monthRec)

    doc.sortMonthsByDate()

    tpl.Execute(w, doc)
  }
}


func main() {
  port := "3000"

  document := newDocument()

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
      json.Unmarshal(byteValue, &document)

    } else {
      fmt.Println("Input file type not recognized")
    }
  } else if len(os.Args) == 1 {
    fmt.Println("No input file: creating empty record")
  }

  fmt.Println("Listening on localhost:"+port)

  // Serve assets folder
  fs := http.FileServer(http.Dir("assets"))

  mux := http.NewServeMux()

  mux.Handle("/assets/", http.StripPrefix("/assets/", fs))

  mux.HandleFunc("/writeJSON", document.writeJson("test_2.json"))

  mux.HandleFunc("/addCategory", document.addCategory())
  mux.HandleFunc("/addWho", document.addPayer())
  mux.HandleFunc("/addCurrency", document.addCurrency())
  mux.HandleFunc("/inputPreviousDebts", document.addPreviousDebts())

  mux.HandleFunc("/changeSheet", document.changeToSheet())

  mux.HandleFunc("/addSheet", document.addSheet())
  mux.HandleFunc("/addEntry", document.addEntry())
  mux.HandleFunc("/", document.indexHandler())

  http.ListenAndServe(":"+port, mux)

  fmt.Println("Listening on localhost:"+port)
}
