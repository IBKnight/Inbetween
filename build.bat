@echo off
setlocal
cd /d "%~dp0"
if not exist bin mkdir bin
echo [1/3] go vet
go vet ./... || exit /b 1
echo [2/3] go test
go test ./... || exit /b 1
echo [3/3] go build
go build -trimpath -o bin\inbetween.exe .\cmd\inbetween || exit /b 1
go build -trimpath -o bin\tracestat.exe .\cmd\tracestat || exit /b 1
echo OK: bin\inbetween.exe, bin\tracestat.exe
