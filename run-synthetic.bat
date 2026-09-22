@echo off
cd /d "%~dp0"
bin\inbetween.exe -source synthetic -view window -size 1280x720 -fps 30 -mult 2 -marker -trace synthetic.csv %*
