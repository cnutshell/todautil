# Introduction

## How to use

```bash
# Compile
go build

./todautil --help
Usage of ./todautil:
  -conf string
        config file
  -path string
        volume path
```

## Example

NB: Before using `todautil`, `toda` must be available. One could check this via `toda --help`.

```bash
./todautil -conf config-examples/latency.json -path /mnt/test/
```

