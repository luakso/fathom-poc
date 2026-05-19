# Project Spec — Fathom

**Version:** v0.1 (draft)
**Author:** Lukas
**Date:** May 2026
**Status:** Committed

---

## One-line description

A weekly published index measuring the flow of money through agent-mediated payments across all 402-style protocols — the canonical source for "what is the agent economy doing right now."

## Why this exists

The agent economy is at the stage where its activity is real but invisible. Nobody can answer basic questions like total monthly flow, category composition, concentration of activity, or growth of supply. The first credible publisher of these numbers tends to win the citation game (cf. Google Trends, Stripe's volume reports, Glassnode for crypto). This project becomes that publisher.

## Strategic frame

This is a **research practice with a data product as the natural artifact, not the goal.** The deliverable is *understanding* of the agent economy, expressed through numbers, charts, and analysis. The numbers are evidence; the product is the framing and conceptual frameworks people start using.

Revenue is downstream of credibility. Credibility comes from consistency over 12+ months.

## v1 scope (locked)

### Data sources (exactly these — no additions in v1)

1. **x402 on Base** — on-chain indexing of facilitator contracts and known settlement addresses
2. **x402 on Solana** — same shape, different chain
3. **Endpoint registry** — hand-curated YAML of known 402-speaking HTTP services, probed periodically for existence, pricing, declared payment methods

### Published metrics (start with exactly these)

1. Total weekly payment volume (USD)
2. Count of active services this week
3. Count of new services this week

That's three numbers. Resist adding more until month two.

### Publication format (locked)

- Weekly cadence, same day every week (pick one in week 1)
- Long-form X post with three charts (one per metric) + two paragraphs of interpretation
- Public GitHub repo with tagged weekly release: JSON snapshot, CSV, Markdown summary
- One long-form Substack/blog piece per month with analysis or methodology

### Tech stack (locked)

- **Language:** Go (everything)
- **Database:** Postgres (single instance)
- **Hosting:** Single VPS, ~$20/month
- **Charts:** Go plotting library, fallback to Python if needed
- **Source control / distribution:** Public GitHub repo
- **No Kubernetes, no Kafka, no managed event store, no Clickhouse**

## Architecture (3 layers)

### Layer 1: Collectors

Independent Go services, one per source. Each:
- Has a cursor (`--from-block X` / `--since T`)
- Writes to shared Postgres
- Fails independently
- Backfillable and replayable

### Layer 2: Warehouse

Postgres with three core tables:
- `payments` — one row per observed payment
- `services` — one row per known 402-speaking service
- `probe_results` — append-only history of endpoint probes

Methodology lives in **versioned SQL views**, never in the raw tables. New methodology = new view (`weekly_payment_volume_v2`), old one preserved for reproducibility.

### Layer 3: Publisher

Go program that:
- Reads weekly views
- Emits JSON + CSV + Markdown to public repo
- Drafts the social post for manual review
- **Does not auto-publish to X** — human in the loop for the first 6 months minimum

## Methodology principles (commit upfront)

1. **Transparency over cleverness.** Every methodology decision is documented in the public repo. If a critic could legitimately question a number, the answer should already be in the methodology doc.

2. **Versioning, not mutation.** Methodology changes create new views. Old views remain runnable. Historical numbers are never silently revised.

3. **Conservative classification.** When in doubt about whether activity is agent-mediated, exclude it. The index loses more credibility from overcounting than from undercounting.

4. **No mixing of synthetic and observed data.** No harness traffic, no simulated flows, no "estimated" volumes from non-observable sources mixed into the published numbers. Observation only.

5. **No editorial cherry-picking.** Bad weeks for the agent economy are published with the same care as good weeks. The audience trusts consistency more than narrative.

## Milestones

### Week 1

- [ ] Repo created (Go monorepo, three service skeletons)
- [ ] VPS provisioned, Postgres running
- [ ] Base x402 collector running and backfilled
- [ ] Hand-curated YAML of ~20 endpoints
- [ ] Endpoint probe service running daily
- [ ] Public data repo created

### Week 2

- [ ] First weekly publication on X (3 metrics, 3 charts, 2 paragraphs)
- [ ] First tagged release in public data repo
- [ ] Solana x402 collector started

### Weeks 3–4

- [ ] Solana metrics added to publication
- [ ] First long-form methodology piece published
- [ ] Visual style for charts locked (palette, fonts, layout)

### Month 2

- [ ] Endpoint registry at 50+ services
- [ ] One composition metric added (volume by category)
- [ ] First decision-relevant analysis piece published

### Month 3

- [ ] Third protocol ingested (L402/Lightning or similar)
- [ ] Reframe public positioning from "x402 index" to "agent economy index"
- [ ] Methodology v2 published with rationale for any changes

### Month 6 (success-check milestone)

Evaluate signals:
- Are people citing the data?
- Are funds, infra companies, journalists reaching out?
- Is the X following growing meaningfully?
- Are custom-cut requests starting to arrive?

If yes → double down, start thinking about paid data feed.
If no → reassess methodology, distribution, or whether the agent economy is growing fast enough yet.

## Non-goals (explicit)

These are NOT part of v1 and not to be built "just in case":

- **The harness.** Park it. Revisit only at month 3+ if needed for methodology validation. Synthetic and published data must remain strictly separate regardless.
- **A web frontend.** GitHub + X is the distribution. Site comes later if ever.
- **A paid API.** The public CSV/JSON is the API for v1.
- **An SDK.** Different project entirely.
- **A sandbox.** Different project entirely.
- **Trading / signals / strategies.** Comes after the index is credible. Year 2 question.
- **User accounts, auth, billing.** Nothing to sell yet.
- **Agent identification / fingerprinting system.** Definitional purity comes later.

## Success metrics

### 90-day signals

- 12+ consecutive weekly publications, zero missed
- 3+ external citations (X, blog, podcast, paper)
- 1+ infrastructure company or fund asks for custom data
- X following growth ≥ 50%

### 12-month signals

- 50+ weekly publications (full year of weekly cadence)
- Cited by at least one major industry report or publication
- Inbound from at least 5 serious commercial conversations
- A clear answer to the question "what's the business shape" — paid feed, research practice, fund, or something else

## Operational discipline

- **Weekly publication is non-negotiable.** If a week looks bad, publish the bad numbers. Never skip.
- **No parallel projects.** The harness is parked. No new side projects until month 6 review.
- **30-minute operability target.** The system should be operable in 30 minutes per week of routine maintenance. If it's taking more, simplify the system.
- **Methodology disputes are good.** Engage them publicly, update the methodology when warranted, never delete history.

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| Agent economy stays too small to be macro-relevant | Index still has value as the first measurement system; pivot framing toward "early indicator" not "macro indicator" |
| Pattern of project-cycling kills consistency | Pre-committed milestones; month-6 explicit review; weekly publication discipline as forcing function |
| Methodology challenged on credibility grounds | Versioned views + open repo + conservative classification principles all designed for this |
| Competitor publishes first | Speed of v1 launch matters; ship week 1 even if minimal |
| Day-job conflict / time constraint | 30-min operability target; scope locked specifically to prevent feature creep |

## The one thing to internalize

The engineering is plumbing. The methodology is the product. The audience-building is the moat.

---

*This spec is the reference document. Re-read it whenever scope creep is tempting. Update it explicitly via a versioned change, never silently.*
