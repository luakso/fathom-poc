// Display formatters. Decimal strings from the artifact parse to floats for
// DISPLAY ONLY — every shown value is rounded far above float error; the
// artifact remains the exact record.
export const num = x => typeof x === "string" ? parseFloat(x) : x;
export const fmtInt = n => Math.round(num(n)).toLocaleString("en-US");
export const fmtMoney = n => { n = num(n);
  if (n >= 1e9) return "$" + (n/1e9).toFixed(2) + "B";
  if (n >= 1e6) return "$" + (n/1e6).toFixed(2) + "M";
  if (n >= 1e3) return "$" + (n/1e3).toFixed(1) + "k";
  return "$" + n.toFixed(2); };
export const fmtMoneyFull = n => "$" + num(n).toLocaleString("en-US",{minimumFractionDigits:2,maximumFractionDigits:2});
export const fmtCount = n => { n = num(n);
  if (n >= 1e6) return (n/1e6).toFixed(2) + "M";
  if (n >= 1e3) return (n/1e3).toFixed(1) + "k";
  return fmtInt(n); };
export const fmtAmt = s => { let t = String(s);
  if (t.includes(".")) t = t.replace(/0+$/,"").replace(/\.$/,"");
  const f = parseFloat(t);
  if (f >= 0.01) return "$" + f.toFixed(f >= 1 ? 2 : Math.min(4, (t.split(".")[1]||"").length || 2));
  return "$" + t; };
export const pct = (a,b,dp=1) => num(b) ? (100*num(a)/num(b)).toFixed(dp) + "%" : "—";

export const BANDDEF = [["dust","<$0.01"],["micro","<$1"],["small","<$100"],["mid","<$1k"],["whale","≥$1k"]];

// Shared verdict vocabulary — panel tags and pin/X-card text must agree, so
// the thresholds live here, in one place, not in each renderer.
// The check is asymmetric: ratios above 1.5× are called out; claims lower than
// measured are uncommon but distinct from "matches". Plain outcome language
// avoids implying a symmetric two-sided test.
export const priceRead = p => p.payee_count < 1000 ? "menu" : p.payee_count >= 5000 ? "market" : "mixed";
export const claimVerdict = ratio =>
  (ratio === null || !isFinite(ratio)) ? "not comparable" :
  ratio >= 1.5 ? `${ratio.toFixed(1)}× higher than measured` :
  ratio < 0.9  ? "lower than measured" :
                 "matches measurement";

// HTML escaper for user-visible text injected into innerHTML.
// claim_text comes from a curated file, but must not be able to break markup.
export const escHtml = s => String(s)
  .replace(/&/g, "&amp;")
  .replace(/</g, "&lt;")
  .replace(/>/g, "&gt;")
  .replace(/"/g, "&quot;");
