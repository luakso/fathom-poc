// Hand-built reliability fixtures. Counts reconcile (windowed<=settlement,
// expired+not_yet_valid<=windowed, rates in [0,1]). The headline is verified-only.
// Numbers illustrative, not production.
function measure(o) {
  return {
    settlement_count: 0, windowed_count: 0, windowed_share: 0,
    cancellation_count: 0, cancellation_rate: 0,
    latency: { p50_s: null, p90_s: null, p99_s: null,
               buckets: { sub1s: 0, "1_10s": 0, "10_60s": 0, "1_10m": 0, gt10m: 0 } },
    expired_count: 0, expired_rate: 0, not_yet_valid_count: 0, not_yet_valid_rate: 0,
    ...o,
  };
}

const allWindow = measure({
  settlement_count: 100, windowed_count: 80, windowed_share: 0.8,
  cancellation_count: 2, cancellation_rate: 0.02,
  latency: { p50_s: 603, p90_s: 604, p99_s: 627,
             buckets: { sub1s: 0, "1_10s": 10, "10_60s": 10, "1_10m": 20, gt10m: 40 } },
  expired_count: 0, expired_rate: 0, not_yet_valid_count: 0, not_yet_valid_rate: 0,
});

export const reliabilityDoc = {
  methodology_version: 1,
  generated_at: "2026-06-22T00:00:00Z",
  data_through_day: "2026-06-06",
  data: {
    windows: { all: allWindow, "7d": structuredClone(allWindow), "30d": structuredClone(allWindow) },
    daily: [
      { day: "2026-06-05", settlement_count: 40, windowed_count: 30, expired_count: 0, not_yet_valid_count: 0, cancellation_count: 1 },
      { day: "2026-06-06", settlement_count: 60, windowed_count: 50, expired_count: 0, not_yet_valid_count: 0, cancellation_count: 1 },
    ],
    cancellation_attribution: {
      by_payer: [{ address: "0xpayer0000000000000000000000000000000000aa", count: 2, facilitator_known: false }],
      by_cancel_from: [{ address: "0xrelayer00000000000000000000000000000000bb", count: 2, facilitator_known: false }],
    },
  },
};

export const economyDoc = {
  methodology_version: 1,
  data: { windows: { all: { txn_count: 100, volume_usdc: "12345.00" } } },
};
