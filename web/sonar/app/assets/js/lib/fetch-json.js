// Shared artifact fetcher. The entity, mechanics, and reliability adapters all
// load a static JSON artifact the same way: no-cache fetch, HTTP guard, JSON
// parse guard, then a structural predicate that must hold before the doc is
// trusted. That logic was copy-pasted verbatim into each adapter; it lives here
// once so the four error messages can never drift apart.
//
// ok(doc) is a structural predicate; fieldHint names the field(s) it checks, so
// a truncated artifact fails with a readable "missing expected fields" message
// rather than a downstream undefined-read. Every throw is a plain Error the
// caller catches and renders in its own fatal screen.
export async function fetchJson(url, ok, fieldHint) {
  let res;
  try { res = await fetch(url, { cache: "no-cache" }); }
  catch (e) { throw new Error(`network error fetching ${url}: ${e.message}`); }
  if (!res.ok) throw new Error(`HTTP ${res.status} fetching ${url}`);
  let doc;
  try { doc = await res.json(); }
  catch (e) { throw new Error(`${url} is not valid JSON: ${e.message}`); }
  if (!ok(doc)) throw new Error(`${url} missing expected fields (${fieldHint})`);
  return doc;
}
