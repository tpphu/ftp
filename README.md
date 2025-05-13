# goftp #

[![Units tests](https://github.com/tpphu/ftp/actions/workflows/unit_tests.yaml/badge.svg)](https://github.com/tpphu/ftp/actions/workflows/unit_tests.yaml)
[![Coverage Status](https://coveralls.io/repos/tpphu/ftp/badge.svg?branch=master&service=github)](https://coveralls.io/github/tpphu/ftp?branch=master)
[![golangci-lint](https://github.com/tpphu/ftp/actions/workflows/golangci-lint.yaml/badge.svg)](https://github.com/tpphu/ftp/actions/workflows/golangci-lint.yaml)
[![CodeQL](https://github.com/tpphu/ftp/actions/workflows/codeql-analysis.yml/badge.svg)](https://github.com/tpphu/ftp/actions/workflows/codeql-analysis.yml)
[![Go ReportCard](https://goreportcard.com/badge/tpphu/ftp)](http://goreportcard.com/report/tpphu/ftp)
[![Go Reference](https://pkg.go.dev/badge/github.com/tpphu/ftp.svg)](https://pkg.go.dev/github.com/tpphu/ftp)

A FTP client package for Go

## Install ##

```
go get -u github.com/tpphu/ftp
```

## Documentation ##

https://pkg.go.dev/github.com/tpphu/ftp

## Example ##

```go
c, err := ftp.Dial("ftp.example.org:21", ftp.DialWithTimeout(5*time.Second))
if err != nil {
    log.Fatal(err)
}

err = c.Login("anonymous", "anonymous")
if err != nil {
    log.Fatal(err)
}

// Do something with the FTP conn

if err := c.Quit(); err != nil {
    log.Fatal(err)
}
```

## Store a file example ##

```go
data := bytes.NewBufferString("Hello World")
err = c.Stor("test-file.txt", data)
if err != nil {
	panic(err)
}
```

## Read a file example ##

```go
r, err := c.Retr("test-file.txt")
if err != nil {
	panic(err)
}
defer r.Close()

buf, err := ioutil.ReadAll(r)
println(string(buf))
```
