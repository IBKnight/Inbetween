@echo off
cd /d "%~dp0"
bin\inbetween.exe -mode eval -size 1280x720 -fps 30 -mult 2 -eval 60 -dump 2 %*
