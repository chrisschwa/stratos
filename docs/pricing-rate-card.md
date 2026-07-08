# Pricing rate card — competitor-mapped (USD, hourly)

Method + concrete rates for the public price plan, benchmarked against published on-demand
prices retrieved **2026-07-08** from official pricing pages (AWS, DigitalOcean, OVHcloud,
Lambda, RunPod). Rates are proposals: apply via the seed payloads in
`deploy/seed/price-plan-seed.json` (or the admin Price plans UI) after sign-off.
Prices are stored in the platform base currency — set base currency **USD** before seeding.

## How the engine models this

One PUBLIC plan, rules keyed `(resourceType, timeUnit=hour)`:

| Rule | resourceType | Filter | Priced attribute | Rate |
|---|---|---|---|---|
| CPU component | instance | — | `vcpus` | $0.008 / vCPU / hr |
| RAM component | instance | — | `ram_gb` | $0.004 / GB / hr |
| GPU per model | instance | `gpu_model eq <model>` | `gpu_count` | table below |
| Public egress | instance_traffic | — | `outgoing_public_traffic_mb` | tier 1: 0 → 1,048,576 MB (1 TiB) = $0; tier 2: beyond = $0.00001/MB ($0.01/GB) |
| Volume | volume | — | `size` | $0.000137 / GB / hr (≈$0.10/GB-mo) |
| Floating IP | floating_ip | — | `existence` | $0.005 / hr |
| Load balancer | load_balancer | — | `existence` | $0.0165 / hr (≈$12/mo) |

GPU rules are ADD_TO_TOTAL **on top of** the CPU/RAM component. RunPod/Lambda headline
prices bundle CPU+RAM into the per-GPU rate, so either (a) accept our small premium
(GPU rate + component), or (b) calibrate each GPU rate down by the flavor's component cost
(e.g. an h100 flavor with 16 vCPU / 128 GB carries 16×0.008 + 128×0.004 = $0.64/hr of
component → set the GPU rate to headline − 0.64). The seed uses (a) with rates already
set ~3–5% under RunPod headline; final calibration = operator decision at seeding.

## CPU VM benchmark (per month, 730 h)

| Shape | Ours | DigitalOcean Basic | OVH b3 | AWS m7i |
|---|---|---|---|---|
| 2 vCPU / 4 GB | $23.4 | $24 | — | ($65 c7i.large≈4GB) |
| 2 vCPU / 8 GB | $35.0 | — | $44 (b3-8) | $74 (m7i.large) |
| 4 vCPU / 8 GB | $46.7 | $48 | — | — |
| 8 vCPU / 16 GB | $93.4 | $96 | — | — |
| 16 vCPU / 32 GB | $186.9 | $192 | — | — |

Formula: vCPU $0.008/hr + RAM $0.004/GB/hr. Root disk bundled ($0) like DO; block storage
is billed separately.

## GPU per-model rates ($/GPU/hr, on-demand)

| gpu_model (alias) | Ours | RunPod | Lambda |
|---|---|---|---|
| rtx-4090 | **0.65** | 0.69 | — |
| rtx-5090 | **0.95** | 0.99 | — |
| l40s | **0.95** | 0.99 | — |
| a40 | **0.42** | 0.44 | — |
| a6000 | **0.47** | 0.49 | 1.09 |
| a100-80gb (PCIe) | **1.35** | 1.39 | 1.99 |
| a100-sxm-80gb | **1.45** | 1.49 | 2.79 |
| h100-pcie | **2.79** | 2.89 | 3.29 |
| h100-sxm | **3.19** | 3.29 | 3.99–4.29 |
| h200 | **4.29** | 4.39 | — |
| b200 | **5.75** | 5.89 | 6.69–6.99 |

Alias vocabulary = the placement trait / pci alias form (`CUSTOM_PCI_A100_80GB` →
`a100-80gb`) — the same names gpu-info capacity and project GPU quota use. A flavor's
model+count derive from `pci_passthrough:alias` extra specs (see `internal/cloud/gpu.go`);
rename the seed rule filters if your aliases differ.

## Other resources — benchmarks

- Volume $0.10/GiB-mo (DO volumes); AWS EBS gp3 $0.08/GB-mo; OVH Classic ≈$0.048/GB-mo.
- Floating IP: AWS public IPv4 $0.005/hr; DO reserved-unattached $5/mo. Ours $0.005/hr flat
  (simple; no attached/unattached split in the billing attributes yet).
- LB: DO from $12/mo; AWS ALB $0.0225/hr + LCU. Ours $0.0165/hr flat.
- Egress: AWS 100 GB free then $0.09/GB; DO pooled 4–8 TiB then $0.01/GiB; OVH free;
  RunPod/Lambda free. Ours: 1 TiB free per server / month, then $0.01/GB (DO-style,
  undercuts AWS hard, still monetizes heavy egress).

Sources: runpod.io/pricing · lambda.ai/service/gpu-cloud · digitalocean.com/pricing
(droplets, load-balancers, volumes, spaces, bandwidth docs) · aws.amazon.com (ec2
on-demand, ebs, vpc, elasticloadbalancing, s3) · us.ovhcloud.com/public-cloud/prices.
OVH LB / additional-IP rows are JS-rendered and were secondary-sourced — reverify before
quoting them externally.

## Seeding

1. Base currency USD (admin → Configuration → Billing; create-form on a fresh install).
2. `deploy/seed/price-plan-seed.json` holds the plan + rule bodies:
   `POST /api/v1/admin/price-plan`, then per rule `POST /api/v1/admin/price-plan/rule`
   (inject the returned plan id into each rule's `pricePlanId`).
3. Verify with `GET /admin/service/{id}/unpriced-flavors` — it lists live flavors that
   match NO enabled public rule (those would bill zero). GPU models without a seed rule
   show up there.
