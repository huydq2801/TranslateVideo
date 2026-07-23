@echo off
title Python OCR Sidecar Server
cd /d "%~dp0"
echo Starting Python OCR Sidecar on http://127.0.0.1:8000...

set PYTHON_CMD=python

where python >nul 2>nul
if %errorlevel% equ 0 goto HAS_PYTHON

if exist "%LOCALAPPDATA%\Programs\Python\Python311\python.exe" goto USE_PY311
if exist "%LOCALAPPDATA%\Programs\Python\Python312\python.exe" goto USE_PY312
if exist "%LOCALAPPDATA%\Programs\Python\Python310\python.exe" goto USE_PY310

echo.
echo [THONG BAO] Chua tim thay Python trong he thong!
echo Vui long tai va cai dat Python tu: https://www.python.org/downloads/
echo Nho tich chon Add Python to PATH khi cai dat.
echo.
pause
exit /b 1

:USE_PY311
set "PYTHON_CMD=%LOCALAPPDATA%\Programs\Python\Python311\python.exe"
goto START_VENV

:USE_PY312
set "PYTHON_CMD=%LOCALAPPDATA%\Programs\Python\Python312\python.exe"
goto START_VENV

:USE_PY310
set "PYTHON_CMD=%LOCALAPPDATA%\Programs\Python\Python310\python.exe"
goto START_VENV

:HAS_PYTHON
set "PYTHON_CMD=python"

:START_VENV
set "VENV_DIR=%TEMP%\comicify_ocr_venv"

if not exist "%VENV_DIR%\Scripts\activate.bat" (
    echo Dang tao Virtual Environment tai %VENV_DIR%...
    %PYTHON_CMD% -m venv "%VENV_DIR%"
)

call "%VENV_DIR%\Scripts\activate.bat"
echo Kiem tra va cap nhat thu vien OCR tu requirements.txt...
pip install -r requirements.txt

:RUN_SERVER
python server.py
pause
