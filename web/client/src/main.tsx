import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { Toaster } from "sonner"
import "./index.css"
import App from "./App"
import { AuthBridge, StratosAuthProvider } from "./lib/auth"

const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: 1, refetchOnWindowFocus: false, staleTime: 15_000 } },
})

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <StratosAuthProvider>
      <AuthBridge>
        <QueryClientProvider client={queryClient}>
          <App />
          <Toaster richColors position="top-right" />
        </QueryClientProvider>
      </AuthBridge>
    </StratosAuthProvider>
  </StrictMode>,
)
