# Apunta

Note down expenses for a period of time

```sh
go build

# See https://docs.openexchangerates.org/docs/
export OPEN_EXCHANGE_APP_ID=<appid>

# New empty record
./apunta

# Open existing file
./apunta path/to/file.json

# Old xlsx compatibility
./apunta path/to/file.xlsx
```


## Testing

```sh
# Run tests
go test

# Run coverage
go test -coverprofile=coverage.out
go tool cover -html=coverage.out

# Benchmarking all benchmarks
go test -bench=.

# Run examples
go test -v
```