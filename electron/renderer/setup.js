document.getElementById('connect').addEventListener('click', async () => {
  const input = document.getElementById('url')
  const errorEl = document.getElementById('error')
  const url = input.value.trim()

  if (!/^https?:\/\/.+/.test(url)) {
    errorEl.textContent = 'http:// 또는 https://로 시작하는 주소를 입력하세요.'
    return
  }

  await window.dbLite.saveServerUrl(url)
})
