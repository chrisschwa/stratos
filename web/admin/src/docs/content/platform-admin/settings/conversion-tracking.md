# Conversion Tracking

The client portal pushes conversion events into the browser's data layer, so you can measure funnel performance in Google Tag Manager and feed the conversions back into Google Ads as campaign goals. Nothing needs configuring server-side — the events fire on their own when the conditions are met; all you do is wire them up in GTM.

## The events that fire

| Event | Fires when | Payload |
|---|---|---|
| `billing_address_filled` | A new customer saves their billing address for the first time. | `{ event: "billing_address_filled" }` |
| `deposit` | A deposit transaction completes successfully in the portal. | `{ event: "deposit", transactionId, conversionValue, conversionCurrency }` |

A sample `deposit` payload:

```json
{
  "event": "deposit",
  "transactionId": "6f1c9a2e-...",
  "conversionValue": 10,
  "conversionCurrency": "USD"
}
```

## How to map them

- Treat `billing_address_filled` as your *begin checkout* equivalent — it marks a signed-up user showing purchase intent before any money moves.
- Treat `deposit` as the purchase/conversion event. `conversionValue` and `conversionCurrency` map straight onto Google Ads conversion-value fields, and `transactionId` deduplicates repeat submissions.

## Wiring it up in Google Tag Manager

1. Install your GTM container on the client portal domain.
2. Create custom-event triggers for `billing_address_filled` and `deposit`.
3. Create tags (e.g. Google Ads conversion tags) that read the data-layer variables `conversionValue`, `conversionCurrency`, and `transactionId`.
4. Import the conversions into Google Ads as campaign goals, so the bidding strategy optimizes on real deposits instead of clicks.

<!-- screenshot: /docs-img/marketing-events-gtm-trigger.png — Google Tag Manager: custom event trigger configured for the deposit event with data layer variables mapped -->
