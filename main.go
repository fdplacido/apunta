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
)


type Document struct {
  PrevDebt      map[string]float64
  Categories    []string
  Payers        []string
  Currencies    []string
  LastUsedCat   string
  LastUsedPayer string
  LastUsedCurr  string
  LastUsedDate  time.Time
  MonthRecs     []MonthRec
}

var (
  tpl = template.Must(template.ParseFiles("index.html"))
  inputFileRead = false
)


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
// Create an empty Document
// *******************************
func newDocument() *Document {
  doc := &Document{}
  // Default values for Document
  doc.Payers = append(doc.Payers, "All")
  doc.Currencies = append(doc.Currencies, "EUR")
  doc.LastUsedDate = time.Now()
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

        // Add entry to the list and sort
        doc.MonthRecs[index].EntryRecords = append(doc.MonthRecs[index].EntryRecords, entry)
        doc.MonthRecs[index].sortRecordsByDate()

        break
      }

      // Check for last item
      if index + 1 == len(doc.MonthRecs) {
        fmt.Printf("Date %s did not fit in any current month", recDate)
      }
    }

    doc.calcAllStats()

    doc.updateLastUsed(entry.Category, entry.PersonName, entry.Currency, entry.Date)

    tpl.Execute(w, doc)
  }
}


// *******************************
// Change active month to selected month
// *******************************
func (doc *Document) updateLastUsed(lastCat, lastPayer, lastCurr string, lastDate time.Time) {
  doc.LastUsedCat = lastCat
  doc.LastUsedPayer = lastPayer
  doc.LastUsedCurr = lastCurr
  doc.LastUsedDate = lastDate
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
    fmt.Println("Saving current data in file named ", fileName)
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

  if len(os.Args) == 2 {
    mux.HandleFunc("/writeJSON", document.writeJson(os.Args[1]))
  } else {
    currentTime := time.Now()
    fileName := "apunta" + currentTime.Format("2006-01-02_150405.json")
    mux.HandleFunc("/writeJSON", document.writeJson(fileName))
  }

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
