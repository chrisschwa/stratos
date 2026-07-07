# Price Plans

A price plan is how Stratos knows what to bill. Think of a plan as a bag of pricing rules; each rule prices a single attribute of a single resource type, per unit of time. On a periodic cycle Stratos rates every cached cloud resource against whichever plan applies and adds the result to the customer's running bill — pure pay-as-you-go.

## Building a plan

Head to **System → Price plans** and create one.

![Create price plan dialog](/docs-img/price-plans-create.png)

| Field | Meaning |
|---|---|
| Name | An internal label, e.g. `Standard pricing`. |
| Access mode | `PUBLIC` applies to every billing profile automatically; `SCOPED` applies only to the profiles you explicitly assign it to. |
| Enabled | A disabled plan is kept but skipped during rating. |

You can clone an existing plan, which duplicates all of its rules — the fastest way to spin up a customer-specific variant of your standard pricing.

### Which plan a customer gets

A billing profile is rated against the `PUBLIC` plan unless a `SCOPED` plan has been assigned to it, in which case the scoped plan wins. Reach for scoped plans when you have negotiated or segment-specific pricing, and let the public plan cover everyone else.

## Pricing rules

Open a plan to work on its rules.

<!-- screenshot: /docs-img/price-plan-rules.png — Price plan detail page listing pricing rules with resource type, priced attribute and time unit columns -->

Each rule holds:

| Field | Meaning |
|---|---|
| Name | e.g. `Instance vCPU hourly`. |
| Resource type | The billable resource the rule targets (see [Resource Types](/docs/platform-admin/billing/resource-types)). |
| Priced attribute | A numeric or boolean attribute of that resource type. A boolean counts as 0 or 1 — pricing the `existence` attribute charges a flat rate simply for the resource being present. |
| Price | The amount charged per attribute unit, per time unit. |
| Time unit | `minute`, `hour`, or `month`. |

<!-- screenshot: /docs-img/price-plan-rule-create.png — Rule creation dialog with a resource type selected and its attribute list expanded -->

The charge for a single billing tick is:

```
charge = price × attribute value        (per time unit)
```

When a plan carries several rules — or one rule prices several attributes — the amounts add up. To hit a monthly target price, divide by the number of time units in a month: at the defaults a month holds 43,200 minutes or 720 hours, and `month`-based rules are charged once, pro-rated across the remaining days when a resource appears mid-cycle. Those divisors are configurable under **System → Billing configuration → Settings → Time unit limits**.

```
Target: a 2-vCPU / 4 GB instance at ~30 €/month, priced hourly
  vCPU rule:  0.0139 €/h × 2 vCPU × 720 h ≈ 20 €
  RAM rule:   0.0035 €/h × 4 GB  × 720 h ≈ 10 €
```

Rules can also carry filters — applying the price only when a resource attribute meets a condition — plus modifiers, both stored alongside the rule.

## Price adjustment rules

On top of per-resource pricing, a plan can hold adjustment rules: tiered surcharges or discounts applied to the already-rated amount. Every tier pairs a starting amount with a modifier (`add` or `subtract`, expressed as a percentage or an absolute value), which lets you model things like volume discounts that deepen as spend climbs. The **Usage** action on an adjustment rule reports how much it has actually added or taken off.
