@echo off
cd /d "E:\CLAUDE AI\OPTM\app"
"E:\CLAUDE AI\_toolchain\go\bin\go.exe" build -o dist\ThazzDraco.exe .
if errorlevel 1 (
    echo ERRO NA COMPILACAO - verifique o codigo
    pause
    exit /b 1
)
start "" "dist\ThazzDraco.exe"