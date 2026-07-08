import { useEffect, useRef, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { loadStripe, type Stripe, type StripeCardElement } from "@stripe/stripe-js"
import { CreditCard as CreditCardIcon, Plus, Star, Trash2 } from "lucide-react"
import { PageHeader } from "@/components/layout/PageHeader"
import { EmptyState } from "@/components/empty-state"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { apiFetch } from "@/lib/api"
import { fmtDate } from "@/lib/format"
import { useBillingSummary, useProjectId } from "@/lib/hooks"
import type { CreditCard } from "@/lib/types"

type Gateway = { id?: string; thirdParty?: string; addCard?: boolean; metadata?: { publicKey?: string } }
type AddCardResponse = { transactionId?: string; externalPaymentId?: string; metadata?: { client_secret?: string } }

export default function CardsPage() {
  const pid = useProjectId()
  const qc = useQueryClient()
  const { data: summary } = useBillingSummary(pid)
  const bp = summary?.id

  const { data: cards, isLoading } = useQuery({
    queryKey: ["cards", bp],
    queryFn: () => apiFetch<CreditCard[]>(`/card/${bp}`),
    enabled: !!bp,
  })
  const { data: gateways } = useQuery({
    queryKey: ["payment-gateways", bp],
    queryFn: () => apiFetch<Gateway[]>(`/payment/${bp}/gateway`),
    enabled: !!bp,
  })
  const gateway = gateways?.find((g) => g.addCard && g.metadata?.publicKey)

  const [addOpen, setAddOpen] = useState(false)
  const [deleting, setDeleting] = useState<CreditCard | null>(null)

  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: ["cards", bp] })
    void qc.invalidateQueries({ queryKey: ["billing-summary", pid] })
  }

  const setDefault = useMutation({
    // POST /card/{bp}/{cardId}/default → updated billing summary (defaultCardId).
    mutationFn: (cardId: string) => apiFetch(`/card/${bp}/${cardId}/default`, { method: "POST" }),
    onSuccess: () => {
      toast.success("Default card updated")
      invalidate()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const remove = useMutation({
    // DELETE /card/{cardId} — the route reuses the {billingProfileId} param slot for the CARD id.
    mutationFn: (cardId: string) => apiFetch(`/card/${cardId}`, { method: "DELETE" }),
    onSuccess: () => {
      toast.success("Card deleted")
      setDeleting(null)
      invalidate()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  return (
    <>
      <PageHeader
        title="Cards"
        description="Saved payment cards on this billing profile."
        actions={
          <Button size="sm" onClick={() => setAddOpen(true)} disabled={!bp || !gateway}>
            <Plus className="size-4" /> Add card
          </Button>
        }
      />

      {isLoading || !bp ? (
        <Skeleton className="h-64" />
      ) : !cards?.length ? (
        <EmptyState
          icon={CreditCardIcon}
          title="No cards yet"
          hint="Add a card to enable deposits and automatic bill collection."
          action={
            <Button onClick={() => setAddOpen(true)} disabled={!gateway}>
              <Plus className="size-4" /> Add card
            </Button>
          }
        />
      ) : (
        <Card className="overflow-hidden py-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Card</TableHead>
                <TableHead>Expires</TableHead>
                <TableHead>Added</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {cards.map((c) => (
                <TableRow key={c.id}>
                  <TableCell className="font-mono">
                    {c.panMasked ?? c.id}
                    {c.id === summary?.defaultCardId ? <Badge className="ml-2">Default</Badge> : null}
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">{fmtDate(c.tokenExpirationDate)}</TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {fmtDate(c.createdAt as string | undefined)}
                  </TableCell>
                  <TableCell className="text-right">
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => setDefault.mutate(c.id)}
                      disabled={c.id === summary?.defaultCardId || setDefault.isPending}
                    >
                      <Star className="size-4" /> Set default
                    </Button>
                    <Button variant="ghost" size="sm" onClick={() => setDeleting(c)}>
                      <Trash2 className="size-4" /> Delete
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Card>
      )}

      {bp && gateway ? (
        <AddCardDialog
          open={addOpen}
          onOpenChange={setAddOpen}
          bp={bp}
          gatewayId={gateway.id ?? ""}
          publicKey={gateway.metadata?.publicKey ?? ""}
          onAdded={invalidate}
        />
      ) : null}

      <Dialog open={!!deleting} onOpenChange={(o) => !o && setDeleting(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete card</DialogTitle>
            <DialogDescription>
              Delete card {deleting?.panMasked ?? deleting?.id}? This cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleting(null)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => deleting && remove.mutate(deleting.id)}
              disabled={remove.isPending}
            >
              {remove.isPending ? "Deleting…" : "Delete card"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}

// Add-card flow: POST /card/{bp}/add {paymentGatewayId} → SetupIntent client_secret →
// Stripe Elements card → stripe.confirmCardSetup → GET the card-confirm callback so the
// API retrieves the SetupIntent and stores the CreditCard.
function AddCardDialog({
  open, onOpenChange, bp, gatewayId, publicKey, onAdded,
}: {
  open: boolean
  onOpenChange: (o: boolean) => void
  bp: string
  gatewayId: string
  publicKey: string
  onAdded: () => void
}) {
  const mountRef = useRef<HTMLDivElement>(null)
  const stripeRef = useRef<Stripe | null>(null)
  const cardRef = useRef<StripeCardElement | null>(null)
  const [setup, setSetup] = useState<{ txnId: string; clientSecret: string } | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (!open) {
      setSetup(null)
      setError(null)
      return
    }
    let cancelled = false
    ;(async () => {
      try {
        const resp = await apiFetch<AddCardResponse>(`/card/${bp}/add`, {
          method: "POST",
          body: { paymentGatewayId: gatewayId },
        })
        const clientSecret = resp?.metadata?.client_secret
        if (!resp?.transactionId || !clientSecret) throw new Error("Gateway did not return a setup secret")
        const stripe = await loadStripe(publicKey)
        if (cancelled) return
        if (!stripe) throw new Error("Stripe failed to load")
        stripeRef.current = stripe
        const card = stripe.elements().create("card")
        cardRef.current = card
        if (mountRef.current) card.mount(mountRef.current)
        setSetup({ txnId: resp.transactionId, clientSecret })
      } catch (e) {
        if (!cancelled) setError(e instanceof Error ? e.message : String(e))
      }
    })()
    return () => {
      cancelled = true
      cardRef.current?.unmount()
      cardRef.current = null
    }
  }, [open, bp, gatewayId, publicKey])

  const confirm = async () => {
    const stripe = stripeRef.current
    const card = cardRef.current
    if (!stripe || !card || !setup) return
    setSaving(true)
    setError(null)
    try {
      const res = await stripe.confirmCardSetup(setup.clientSecret, { payment_method: { card } })
      if (res.error) throw new Error(res.error.message ?? "Card confirmation failed")
      // Finalize server-side (whitelisted callback: retrieves the SetupIntent + stores the card,
      // returns 200; throws here if the server couldn't finalize).
      await apiFetch(`/callbacks/payment/stripe/card/confirm/${setup.txnId}`)
      toast.success("Card added")
      onAdded()
      onOpenChange(false)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add card</DialogTitle>
          <DialogDescription>Card details go directly to Stripe — they never touch Stratos.</DialogDescription>
        </DialogHeader>
        <div className="rounded-md border p-3">
          <div ref={mountRef} />
        </div>
        {error ? <p className="text-sm text-destructive">{error}</p> : null}
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={() => void confirm()} disabled={!setup || saving}>
            {saving ? "Saving…" : "Save card"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
