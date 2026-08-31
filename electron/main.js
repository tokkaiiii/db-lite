// db-lite 데스크톱 셸의 메인 프로세스.
//
// 이 앱은 Go 서버를 내장하지 않는다 — 첫 실행 시 입력받아 저장한
// 서버 주소로 BrowserWindow를 띄워 기존 웹 UI에 접속만 한다.
// (ADR 0010: docs/adr/0010-electron-thin-client-shell.md)
const { app, BrowserWindow, ipcMain } = require('electron')
const path = require('path')
const fs = require('fs')

const configPath = path.join(app.getPath('userData'), 'config.json')

function readServerUrl() {
  try {
    const raw = fs.readFileSync(configPath, 'utf-8')
    return JSON.parse(raw).serverUrl || null
  } catch {
    return null
  }
}

function writeServerUrl(url) {
  fs.writeFileSync(configPath, JSON.stringify({ serverUrl: url }, null, 2))
}

function createMainWindow(serverUrl) {
  const win = new BrowserWindow({
    width: 1280,
    height: 800,
    webPreferences: {
      contextIsolation: true,
    },
  })
  win.loadURL(serverUrl)
}

function createSetupWindow() {
  const win = new BrowserWindow({
    width: 480,
    height: 340,
    resizable: false,
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
    },
  })
  win.loadFile(path.join(__dirname, 'renderer', 'setup.html'))
  return win
}

ipcMain.handle('db-lite:save-server-url', (event, url) => {
  writeServerUrl(url)
  const setupWin = BrowserWindow.fromWebContents(event.sender)
  setupWin?.close()
  createMainWindow(url)
})

app.whenReady().then(() => {
  const savedUrl = readServerUrl()
  if (savedUrl) {
    createMainWindow(savedUrl)
  } else {
    createSetupWindow()
  }
})

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') app.quit()
})
