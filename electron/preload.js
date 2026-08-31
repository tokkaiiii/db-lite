const { contextBridge, ipcRenderer } = require('electron')

contextBridge.exposeInMainWorld('dbLite', {
  saveServerUrl: (url) => ipcRenderer.invoke('db-lite:save-server-url', url),
})
