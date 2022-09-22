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
      root of mount point
```

## Example

NB: Before using `todautil`, `toda` must be available. One could check this via `toda --help`.

```bash
./todautil -conf config/latency.json -path /mnt/d
```

