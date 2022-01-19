package main

import (
	"testing"
	"fmt"
	"time"
)

// Testing
func TestIsSameMonthYear(t *testing.T) {

	date_a := time.Date(2021, time.Month(5), 24, 1, 2, 3, 4, time.Now().Location())
	date_b := time.Date(2021, time.Month(5), 13, 5, 6, 7, 8, time.Now().Location())
	is_same := isSameMonthYear(date_a, date_b)

	if is_same == false {
		t.Errorf("Year and Month are different")
	}
}

// Benchmarking
func BenchmarkIsSameMonthYear(b *testing.B) {
	for i := 0; i < b.N; i++ {
		date_a := time.Date(2021, time.Month(5), 24, 1, 2, 3, 4, time.Now().Location())
		date_b := time.Date(2021, time.Month(5), 13, 5, 6, 7, 8, time.Now().Location())
		isSameMonthYear(date_a, date_b)
	}
}

// Examples
func ExampleIsSameMonthYear() {
		date_a := time.Date(2021, time.Month(5), 24, 1, 2, 3, 4, time.Now().Location())
		date_b := time.Date(2021, time.Month(5), 13, 5, 6, 7, 8, time.Now().Location())
    fmt.Println(isSameMonthYear(date_a, date_b))
    // Output: true
}