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
  ExchRate   float64
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
  StartDate     time.Time
  GroupName     string
  ActiveGroup   bool
  AvgExchRates  []ExRateEntry
  Stats         MonthStats
  EntryRecords  []EntryRec
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
// Get and calculate all exchange rates for this month
// *******************************
func (month *MonthRec) ExchRatesCalcs() []ExRateEntry {

  // Map of currencies to map of dates - exchRates
  checked_entries := map[string]map[time.Time]float64{}

  for index, entryRec := range month.EntryRecords {

    if entryRec.Currency != "EUR" {

      // Check if currency was already seen
      if dates_map, curr_ok := checked_entries[entryRec.Currency]; curr_ok {

        // check if date was already seen
        if _, date_ok := dates_map[entryRec.Date]; date_ok {
          break
        } else {
          // Get exchage rate
          if entryRec.ExchRate == 0.0 {
            rate, err := exchRates.GetRate(entryRec.Currency, "EUR", entryRec.Date)
            if err != nil {
              fmt.Println(err)
            }
            // Set rate for future use in entry
            entryRec.ExchRate = rate
            month.EntryRecords[index] = entryRec

            // Save to checked entries
            dates_map[entryRec.Date] = entryRec.ExchRate
            checked_entries[entryRec.Currency] = dates_map
          } else {
            dates_map[entryRec.Date] = entryRec.ExchRate
            checked_entries[entryRec.Currency] = dates_map
          }
        }

      } else {

        dates_map := map[time.Time]float64{}
        // Create new entry, get exchage rate
        if entryRec.ExchRate == 0.0 {
          rate, err := exchRates.GetRate(entryRec.Currency, "EUR", entryRec.Date)
          if err != nil {
            fmt.Println(err)
          }
          // Set rate for future use in entry
          entryRec.ExchRate = rate
          month.EntryRecords[index] = entryRec

          // Save to checked entries
          dates_map[entryRec.Date] = entryRec.ExchRate
          checked_entries[entryRec.Currency] = dates_map
        } else {
          dates_map[entryRec.Date] = entryRec.ExchRate
          checked_entries[entryRec.Currency] = dates_map
        }
      }

    }
  }

  // Calculate average per currency
  avg_curr := map[string]float64{}
  for curr, dates_map := range checked_entries {
    num_elems := len(dates_map)
    for _, exch := range dates_map {
      avg_curr[curr] += exch
    }
    avg_curr[curr] = avg_curr[curr]/float64(num_elems)
  }


  // set the caulcated exchange rates to the month
  for curr, avg_val := range avg_curr {
    new_rate := ExRateEntry{"EUR", curr, avg_val}
    month.AvgExchRates = append(month.AvgExchRates, new_rate)
  }

  return month.AvgExchRates

}



// *******************************
// Get the active month and calculate its exchange rate
// *******************************
func (doc *Document) calcExchRate() http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {

    for index, month := range doc.MonthRecs {
      if month.ActiveGroup {
        doc.MonthRecs[index].AvgExchRates = month.ExchRatesCalcs()
        break
      }
    }

    doc.calcAllStats()

    tpl.Execute(w, doc)

  }
}






// *******************************
// Calculate average exchange rate for this month
// *******************************
func (month *MonthRec) getAvgExchRates() []ExRateEntry {

  ocurrences := map[string]int{}
  accumulated := map[string]float64{}
  for _, entryRec := range month.EntryRecords {
    ocurrences[entryRec.Currency] += 1
    accumulated[entryRec.Currency] += entryRec.ExchRate
  }

  avg_entries := make([]ExRateEntry, 0)
  for key, value := range ocurrences {
    if key != "EUR" {
      avg_val := accumulated[key]/float64(value)
      rate_entry := ExRateEntry{key, "EUR", avg_val}
      avg_entries = append(avg_entries, rate_entry)
    }
  }

  return avg_entries
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

    // Set value for exchange rate
    rate_val := 1.0
    for _, month_rate := range month.AvgExchRates {
      if month_rate.CurrFrom == dayRec.Currency {
        rate_val = month_rate.AvgVal
      }
    }

    // Store special case for all to process at the end
    // Skip special case statistics
    if dayRec.PersonName == "B" || dayRec.PersonName == "All" {
      allSpent = append(allSpent, dayRec.Amount * rate_val)
      continue
    }

    if stats, ok := month.Stats.AllPayersStats[dayRec.PersonName]; ok {
      // Key already thare, add
      stats.Spent += (dayRec.Amount * rate_val)
      month.Stats.AllPayersStats[dayRec.PersonName] = stats
    } else {
      // no key, just create the value
      stats = PayerStats{(dayRec.Amount * rate_val), 0.0, 0.0}
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
  monthRecord.AvgExchRates = make([]ExRateEntry, 0)

  return monthRecord
}


// *******************************
// Create an empty Document
// *******************************
func newDocument() *Document {
  doc := &Document{}
  // Default values for Document
  doc.Payers = append(doc.Payers, "All")
  doc.Currencies = append(doc.Currencies, "EUR")
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

    // TODO check that length is 3 and capital letters
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
      doc.MonthRecs[index].Stats = month.calcStats(nil, doc.PrevDebt)
    } else {
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

        if entry.Currency == "EUR" {
          entry.ExchRate = 1.0
        } else {
          entry.ExchRate = 0.0
        }

        // // Check if currency is different from base one (EUR)
        // if entry.Currency != "EUR" {
        //   fmt.Println("entry not EUR")
        //   // Check if an entry with same date has an exchange rate already
        //   for _, entryExch := range month.EntryRecords {
        //     if  entryExch.Date.Day() == entry.Date.Day() {
        //       if entryExch.ExchRate != 0.0 {
        //         entry.ExchRate = entryExch.ExchRate
        //         break
        //       }
        //     }
        //   }

        //   // Get new exchange rate from API
        //   if entry.ExchRate == 0.0 {
        //     rate, err := exchRates.GetRate(entry.Currency, "EUR", entry.Date)
        //     if err != nil {
        //       fmt.Println(err)
        //     }
        //     entry.ExchRate = rate
        //     fmt.Println("Got exchange rate: ", entry.ExchRate)
        //   }

        // } else {
        //   entry.ExchRate = 1.0
        // }

        // Add entry to the list and sort
        doc.MonthRecs[index].EntryRecords = append(doc.MonthRecs[index].EntryRecords, entry)
        doc.MonthRecs[index].sortRecordsByDate()

        // // Recalculate month average exchange rates with new entry
        // if entry.Currency != "EUR" {
        //   doc.MonthRecs[index].AvgExchRates = doc.MonthRecs[index].getAvgExchRates()
        // }

        break
      }

      // Check for last item
      if index + 1 == len(doc.MonthRecs) {
        fmt.Printf("Date %s did not fit in any current month", recDate)
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

    doc.markMonthAsActive(selectedSheet)

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
func (doc *Document) markMonthAsActive(name string) {
  for index, month := range doc.MonthRecs {
    if name != month.GroupName {
      month.ActiveGroup = false
      doc.MonthRecs[index] = month
    } else {
      month.ActiveGroup = true
      doc.MonthRecs[index] = month
    }
  }
}


// *******************************
// Add sheet given a name
// *******************************
func (doc *Document) addSheet() http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {

    defer tpl.Execute(w, doc)

    monthRec := newMonthRec()

    inputName := strings.TrimSpace(r.FormValue("sheetName"))

    // Check if name was already used
    for _, month := range doc.MonthRecs {
      if inputName == month.GroupName {
        fmt.Printf("Name %s was already used.", inputName)
        return
      }
    }
    monthRec.GroupName = inputName

    // If no input is provided, use date
    if monthRec.GroupName == "" {
      selectedMonthYear := r.FormValue("monthYearSheet")
      // Check if name was already used
      for _, month := range doc.MonthRecs {
        if selectedMonthYear == month.GroupName {
          fmt.Sprintf("Name %s was already used.", inputName)
          return
        }
      }
      monthRec.GroupName = selectedMonthYear
    }

    // Create starting date for new month
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

    // Mark new month as active
    doc.markMonthAsActive(monthRec.GroupName)

    // Add it do the document and sort months
    doc.MonthRecs = append(doc.MonthRecs, *monthRec)
    doc.sortMonthsByDate()
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
  mux.HandleFunc("/calcExchRateMonth", document.calcExchRate())

  mux.HandleFunc("/addEntry", document.addEntry())
  mux.HandleFunc("/", document.indexHandler())

  http.ListenAndServe(":"+port, mux)

  fmt.Println("Listening on localhost:"+port)
}
