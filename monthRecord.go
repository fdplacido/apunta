package main

import (
  "fmt"
  "time"
  "sort"

  "apunta/exchRates"
)

const (
  concurrencyLevel = 10
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
// Sort records by ascending date within a month
// *******************************
func (month *MonthRec) sortRecordsByDate() {
  sort.SliceStable(month.EntryRecords, func(i, j int) bool {
    return month.EntryRecords[i].Date.Before(month.EntryRecords[j].Date)
  })
}


// *******************************
// Get and calculate all exchange rates for this month
// *******************************
func (month *MonthRec) ExchRatesCalcs() []ExRateEntry {

  // Map of non-EUR currencies to map of dates - indexes in the entry records
  checked_entries := map[string]map[time.Time]int{}
  same_date_entries := []int{}
  for index, entryRec := range month.EntryRecords {
    if entryRec.Currency != "EUR" {
      // Check if currency was already seen
      if dates_map, curr_ok := checked_entries[entryRec.Currency]; curr_ok {
        // check if date was already seen
        if _, date_ok := dates_map[entryRec.Date]; date_ok {
          same_date_entries = append(same_date_entries, index)
          continue
        } else {
          dates_map[entryRec.Date] = index
          checked_entries[entryRec.Currency] = dates_map
        }
      } else {
        // Add new currency with the date and rate
        dates_map := map[time.Time]int{}
        dates_map[entryRec.Date] = index
        checked_entries[entryRec.Currency] = dates_map
      }
    }
  }

  // Get rates from API in parallel
  queue := make(chan bool, concurrencyLevel)
  for curr, map_dates := range checked_entries {
    for date, index := range map_dates {
      // Avoid asking again for the rate
      if month.EntryRecords[index].ExchRate == 0.0 {
        // Fill channel with dummy flags
        queue <- true
        go func(_curr string, _date time.Time, _index int) {
          // clear the flag after finishing
          defer func() { <- queue }()
          rate, err := exchRates.GetRate(_curr, "EUR", _date)
          if err != nil {
            fmt.Println(err)
          }
          month.EntryRecords[_index].ExchRate = rate
        }(curr, date, index)
      }
    }
  }

  // flush the queue to compute last values
  for i := 0; i < concurrencyLevel; i++ {
    queue <- true
  }

  // Set repeated days entries rates
  for _, index := range same_date_entries {
    downloaded_rate_idx := checked_entries[month.EntryRecords[index].Currency][month.EntryRecords[index].Date]
    month.EntryRecords[index].ExchRate = month.EntryRecords[downloaded_rate_idx].ExchRate
  }

  // Calculate average per currency
  avg_curr := map[string]float64{}
  for curr, map_dates := range checked_entries {
    num_elems := len(map_dates)
    for _, index := range map_dates {
      avg_curr[curr] += month.EntryRecords[index].ExchRate
    }
    avg_curr[curr] = avg_curr[curr]/float64(num_elems)
  }

  // set the caulcated exchange rates to the month
  for curr, avg_val := range avg_curr {
    // Check if rate was already set
    for i, saved_avg_rate := range month.AvgExchRates {
      if saved_avg_rate.CurrFrom == curr {
        month.AvgExchRates[i].AvgVal = avg_val
      } else {
        new_rate := ExRateEntry{curr, "EUR", avg_val}
        month.AvgExchRates = append(month.AvgExchRates, new_rate)
      }
    }
  }

  return month.AvgExchRates
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
    for prev_payer, prev_stats := range prevMonth.Stats.AllPayersStats {
      month.Stats.AllPayersStats[prev_payer] = PayerStats{0.0, (-1 * prev_stats.Debt), 0.0}
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