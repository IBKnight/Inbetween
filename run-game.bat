@echo off
cd /d "%~dp0"
if "%~1"=="" (
  echo usage: run-game.bat "part of the game window title" [extra flags]
  echo window list: bin\inbetween.exe -mode list
  exit /b 2
)
set TITLE=%~1
shift
bin\inbetween.exe -window "%TITLE%" -delay 3s -mult 2 -trace game.csv %1 %2 %3 %4 %5 %6 %7 %8 %9
