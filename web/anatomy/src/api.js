async function getJSON(url) {
  const res = await fetch(url)
  const body = await res.json()
  if (!res.ok) throw new Error(body?.error || `request failed: ${res.status}`)
  return body
}

export function fetchTx(chain, hash) {
  return getJSON(`/api/tx/${chain}/${hash}`)
}

export function fetchStats(chain, addr) {
  return getJSON(`/api/address/${chain}/${addr}/stats`)
}
