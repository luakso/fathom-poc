// Tiny shared series helpers — chart annotations and pin text must compute
// the same numbers from the same series, so the math lives in one place.
export const medianOf = a => [...a].sort((x, y) => x - y)[Math.floor(a.length / 2)] ?? 0;
export const peakIndex = a => a.reduce((pi, v, i) => (v > a[pi] ? i : pi), 0);
