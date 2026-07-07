import { useEffect } from "react"
import { useAuth } from "@/lib/auth"
import { Button } from "@/components/ui/button"

// Public landing: one job — sign in. Night-sky brand moment with the horizon
// gradient as the only decoration.
export function LoginPage() {
  const auth = useAuth()

  useEffect(() => {
    document.title = "Stratos Console"
  }, [])

  return (
    <div className="flex min-h-screen flex-col items-center justify-center bg-[#0b1220] px-6 text-white">
      <div className="w-full max-w-sm">
        <h1 className="font-display text-4xl font-semibold tracking-tight">
          Stratos<span className="text-[#22d3ee]">.</span>
        </h1>
        <p className="mt-3 text-sm text-white/60">
          Compute, networking, storage and billing for your cloud — in one console.
        </p>
        <div className="horizon mt-6" />
        <Button
          className="mt-8 w-full bg-[#4f46e5] text-white hover:bg-[#4338ca]"
          size="lg"
          disabled={auth.isLoading}
          onClick={() => void auth.signinRedirect()}
        >
          {auth.isLoading ? "Checking session…" : "Sign in"}
        </Button>
        <p className="mt-4 text-center text-xs text-white/40">
          New here? Sign-up happens on the sign-in page.
        </p>
      </div>
    </div>
  )
}
