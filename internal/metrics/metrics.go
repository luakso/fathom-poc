// Package metrics builds and emits the precomputed dashboard artifacts. The
// rollup cube (metrics_daily_v2) is a full recompute from payment_x402_v1;
// the emit step turns it into static JSON. Nothing here is on the collector's
// write path.
package metrics
