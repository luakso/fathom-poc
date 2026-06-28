// Hand-built mechanics fixtures. known+unknown==settlement, batch payment_count
// sums to settlement, pct_batched in [0,1]. tx_type sums to 9 (< settlement 10 →
// 1 "other"); selector_mix is illustrative top-N. Numbers not from production.
function measure(o) {
  return {
    settlement_count: 0,
    fee: { tx_type: { "0": 0, "1": 0, "2": 0 }, max_fee: { p50: null, p90: null, p99: null }, max_priority: { p50: null, p90: null, p99: null } },
    auth_window_width: { count: 0, p50_s: null, p90_s: null, p99_s: null },
    over_provisioning: { count: 0, ratio_p50: null, ratio_p90: null, ratio_p99: null },
    tx_value_nonzero: 0,
    hygiene: { dup_auth_nonce: 0, same_block_replay: 0 },
    selector_mix: [],
    ...o,
  };
}

const allWindow = {
  ...measure({
    settlement_count: 10,
    fee: { tx_type: { "0": 1, "1": 0, "2": 8 }, max_fee: { p50: "1000", p90: "2000", p99: "3000" }, max_priority: { p50: "100", p90: "200", p99: "300" } },
    auth_window_width: { count: 8, p50_s: 900, p90_s: 1800, p99_s: 4200 },
    over_provisioning: { count: 9, ratio_p50: 0.9, ratio_p90: 0.98, ratio_p99: 0.99 },
    tx_value_nonzero: 0,
    hygiene: { dup_auth_nonce: 0, same_block_replay: 0 },
    selector_mix: [
      { selector_hex: "e3ee160e", settlement_kind: "transfer", txn_count: 6, volume_usdc: "60.00" },
      { selector_hex: "82ad56cb", settlement_kind: "transfer", txn_count: 3, volume_usdc: "30.00" },
      { selector_hex: "deadbeef", settlement_kind: "transfer", txn_count: 1, volume_usdc: "10.00" },
    ],
  }),
  batch: {
    histogram: [
      { bucket: "1", tx_count: 7, payment_count: 7 },
      { bucket: "2-10", tx_count: 1, payment_count: 3 },
    ],
    pct_batched: 0.3,
    max_batch_size: 3,
  },
  block_density: { max_per_block: 4, p99_per_block: 4, mean_per_block: 2.0, distinct_blocks: 5 },
  cost: { txn_count: 10, gas_eth: "0.01", gas_eth_l1: "0.001", gas_eth_l2: "0.009", gas_usd: "20.00", gas_cents_per_dollar: "0.0089", breakeven_txn_count: 5 },
};

export const mechanicsDoc = {
  methodology_version: 1,
  generated_at: "2026-06-22T00:00:00Z",
  data_through_day: "2026-06-06",
  data: { windows: { all: allWindow, "7d": structuredClone(allWindow), "30d": structuredClone(allWindow) } },
};

export const economyDoc = {
  methodology_version: 1,
  data: { windows: { all: { txn_count: 10, volume_usdc: "100.00" } } },
};
