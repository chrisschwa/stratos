import { useAuth } from "@/lib/auth"
import { Button } from "@/components/ui/button"

export function LoginPage() {
  const auth = useAuth()
  return (
    <div className="flex min-h-screen flex-col items-center justify-center bg-[#1e1b4b] px-6 text-white">
      <div className="w-full max-w-sm">
        <h1 className="font-display text-4xl font-semibold tracking-tight">
          Stratos<span className="text-[#f59e0b]">.</span> <span className="text-lg font-normal text-white/50">admin</span>
        </h1>
        <p className="mt-3 text-sm text-white/60">Operate the platform: customers, billing, cloud providers.</p>
        <div className="horizon mt-6" />
        <Button
          className="mt-8 w-full bg-[#f59e0b] text-[#2a1a02] hover:bg-[#d97706]"
          size="lg"
          disabled={auth.isLoading}
          onClick={() => void auth.signinRedirect()}
        >
          {auth.isLoading ? "Checking session…" : "Sign in"}
        </Button>
      </div>
    </div>
  )
}
