// Item 8.4 — format.js unit tests.
// Covers: fmtAmt decimal branches, priceRead boundaries, claimVerdict (all
// paths including zero-measured guard), escHtml, fmtMoney/fmtCount/fmtInt
// edge cases.
import { describe, it, expect } from "vitest";
import {
  fmtAmt,
  fmtMoney,
  fmtCount,
  fmtInt,
  priceRead,
  claimVerdict,
  escHtml,
  num,
  pct,
} from "../app/assets/js/format.js";

// ---------------------------------------------------------------------------
// fmtAmt
// ---------------------------------------------------------------------------
describe("fmtAmt — decimal branches", () => {
  it("whole dollar amount shows 2 decimal places", () => {
    expect(fmtAmt("1")).toBe("$1.00");
    expect(fmtAmt("100")).toBe("$100.00");
  });

  it("amount with cents: 2 dp when value >= $1", () => {
    expect(fmtAmt("1.50")).toBe("$1.50");
    expect(fmtAmt("99.99")).toBe("$99.99");
  });

  it("sub-$1 amount: decimal places match significant input digits", () => {
    // "0.01" has 2 decimal places → "$0.01"
    expect(fmtAmt("0.01")).toBe("$0.01");
    // "0.1" has 1 decimal place → "$0.1"
    expect(fmtAmt("0.1")).toBe("$0.1");
  });

  it("sub-$1 amount: up to 4 significant decimal places", () => {
    // "0.0001" has 4 decimal places, all significant
    expect(fmtAmt("0.0001")).toBe("$0.0001");
  });

  it("strips trailing zeros from the decimal part before counting dp", () => {
    // "0.10" → strip trailing zero → "0.1" → 1 decimal place → $0.1
    expect(fmtAmt("0.10")).toBe("$0.1");
    // "1.50" → strip trailing zero → "1.5" → >= $1 → .toFixed(2) = $1.50
    expect(fmtAmt("1.50")).toBe("$1.50");
  });

  it("sub-penny amount: returns $ + raw string (no further rounding)", () => {
    // f < 0.01 branch — raw string preserved
    expect(fmtAmt("0.001")).toBe("$0.001");
    expect(fmtAmt("0.0001")).toBe("$0.0001");
    expect(fmtAmt("0.00001")).toBe("$0.00001");
  });

  it("numeric input (not string) works the same as string", () => {
    expect(fmtAmt(1)).toBe("$1.00");
    expect(fmtAmt(0.001)).toBe("$0.001");
  });

  it("large values use 2 dp when >= $1", () => {
    expect(fmtAmt("999.99")).toBe("$999.99");
  });
});

// ---------------------------------------------------------------------------
// fmtMoney
// ---------------------------------------------------------------------------
describe("fmtMoney — edge cases", () => {
  it("zero renders as $0.00", () => {
    expect(fmtMoney(0)).toBe("$0.00");
  });

  it("sub-$1k: two decimal places", () => {
    expect(fmtMoney(500.5)).toBe("$500.50");
  });

  it("thousands: uses k suffix with one decimal", () => {
    expect(fmtMoney(1000)).toBe("$1.0k");
    expect(fmtMoney(5500)).toBe("$5.5k");
  });

  it("millions: uses M suffix with two decimals", () => {
    expect(fmtMoney(1_000_000)).toBe("$1.00M");
    expect(fmtMoney(7_538_241)).toMatch(/^\$7\.\d{2}M$/);
  });

  it("billions: uses B suffix", () => {
    expect(fmtMoney(2_000_000_000)).toBe("$2.00B");
  });

  it("accepts string input via num()", () => {
    expect(fmtMoney("500.00")).toBe("$500.00");
  });
});

// ---------------------------------------------------------------------------
// fmtCount
// ---------------------------------------------------------------------------
describe("fmtCount — edge cases", () => {
  it("zero renders as '0'", () => {
    expect(fmtCount(0)).toBe("0");
  });

  it("sub-1k: integer string", () => {
    expect(fmtCount(999)).toBe("999");
  });

  it("thousands: one decimal + k", () => {
    expect(fmtCount(1500)).toBe("1.5k");
    expect(fmtCount(10000)).toBe("10.0k");
  });

  it("millions: two decimal + M", () => {
    expect(fmtCount(1_000_000)).toBe("1.00M");
    expect(fmtCount(21_188_494)).toMatch(/^\d+\.\d{2}M$/);
  });

  it("accepts string input", () => {
    expect(fmtCount("2000")).toBe("2.0k");
  });
});

// ---------------------------------------------------------------------------
// fmtInt
// ---------------------------------------------------------------------------
describe("fmtInt — edge cases", () => {
  it("zero renders as '0'", () => {
    expect(fmtInt(0)).toBe("0");
  });

  it("rounds float to nearest integer before formatting", () => {
    expect(fmtInt(1000.7)).toBe("1,001");
    expect(fmtInt(999.4)).toBe("999");
  });

  it("large number uses locale thousands separator", () => {
    expect(fmtInt(21188494)).toBe("21,188,494");
  });

  it("accepts string input", () => {
    expect(fmtInt("1000")).toBe("1,000");
  });
});

// ---------------------------------------------------------------------------
// priceRead — boundary tests
// ---------------------------------------------------------------------------
describe("priceRead — boundaries", () => {
  it("payee_count < 1000 → 'menu'", () => {
    expect(priceRead({ payee_count: 999 })).toBe("menu");
    expect(priceRead({ payee_count: 1 })).toBe("menu");
    expect(priceRead({ payee_count: 0 })).toBe("menu");
  });

  it("payee_count === 1000 → 'mixed' (boundary: not < 1000)", () => {
    expect(priceRead({ payee_count: 1000 })).toBe("mixed");
  });

  it("payee_count 1001–4999 → 'mixed'", () => {
    expect(priceRead({ payee_count: 2500 })).toBe("mixed");
    expect(priceRead({ payee_count: 4999 })).toBe("mixed");
  });

  it("payee_count === 5000 → 'market' (boundary: >= 5000)", () => {
    expect(priceRead({ payee_count: 5000 })).toBe("market");
  });

  it("payee_count > 5000 → 'market'", () => {
    expect(priceRead({ payee_count: 10000 })).toBe("market");
  });
});

// ---------------------------------------------------------------------------
// claimVerdict — all paths
// ---------------------------------------------------------------------------
describe("claimVerdict — all paths", () => {
  it("null ratio → 'not comparable'", () => {
    expect(claimVerdict(null)).toBe("not comparable");
  });

  it("non-finite ratio (Infinity) → 'not comparable'", () => {
    // When measured_value === 0, caller passes null (adapter); but claimVerdict
    // itself guards !isFinite too — test the guard directly.
    expect(claimVerdict(Infinity)).toBe("not comparable");
    expect(claimVerdict(NaN)).toBe("not comparable");
  });

  it("ratio >= 1.5 → 'N.N× higher than measured'", () => {
    expect(claimVerdict(1.5)).toBe("1.5× higher than measured");
    expect(claimVerdict(2.0)).toBe("2.0× higher than measured");
    expect(claimVerdict(10)).toBe("10.0× higher than measured");
  });

  it("ratio exactly 1.5 is '× higher' (boundary: >= 1.5)", () => {
    expect(claimVerdict(1.5)).toContain("higher than measured");
  });

  it("ratio < 0.9 → 'lower than measured'", () => {
    expect(claimVerdict(0.0)).toBe("lower than measured");
    expect(claimVerdict(0.5)).toBe("lower than measured");
    expect(claimVerdict(0.89)).toBe("lower than measured");
  });

  it("ratio exactly 0.9 → 'matches measurement' (boundary: not < 0.9)", () => {
    expect(claimVerdict(0.9)).toBe("matches measurement");
  });

  it("ratio 0.9–1.49 → 'matches measurement'", () => {
    expect(claimVerdict(1.0)).toBe("matches measurement");
    expect(claimVerdict(1.49)).toBe("matches measurement");
  });
});

// ---------------------------------------------------------------------------
// escHtml
// ---------------------------------------------------------------------------
describe("escHtml — HTML special characters", () => {
  it("escapes ampersand", () => {
    expect(escHtml("a & b")).toBe("a &amp; b");
  });

  it("escapes less-than", () => {
    expect(escHtml("<script>")).toBe("&lt;script&gt;");
  });

  it("escapes double-quote", () => {
    expect(escHtml('"hello"')).toBe("&quot;hello&quot;");
  });

  it("leaves plain text untouched", () => {
    expect(escHtml("no special chars")).toBe("no special chars");
  });

  it("coerces non-string input to string before escaping", () => {
    expect(escHtml(42)).toBe("42");
    expect(escHtml(null)).toBe("null");
  });
});

// ---------------------------------------------------------------------------
// num — string-to-float passthrough
// ---------------------------------------------------------------------------
describe("num — string/number coercion", () => {
  it("numeric input is returned as-is", () => {
    expect(num(42)).toBe(42);
    expect(num(0)).toBe(0);
  });

  it("string input is parsed as float", () => {
    expect(num("1.5")).toBe(1.5);
    expect(num("0.001000")).toBe(0.001);
  });
});

// ---------------------------------------------------------------------------
// pct — percentage helper
// ---------------------------------------------------------------------------
describe("pct — edge cases", () => {
  it("returns formatted percentage with default 1 dp", () => {
    expect(pct(50, 100)).toBe("50.0%");
  });

  it("returns '—' when denominator is 0", () => {
    expect(pct(1, 0)).toBe("—");
  });

  it("respects custom dp argument", () => {
    expect(pct(1, 3, 2)).toBe("33.33%");
  });
});
