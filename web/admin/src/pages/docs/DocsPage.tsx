// Public documentation viewer (/docs/*). Markdown-driven: pages live in
// src/docs/content/**/*.md, the sidebar comes from src/docs/manifest.ts.
// Editing docs = editing markdown; no code changes needed for new pages
// beyond a manifest entry.
import { useEffect, useState } from "react"
import { Link, useParams } from "react-router-dom"
import ReactMarkdown from "react-markdown"
import remarkGfm from "remark-gfm"
import { docsTitle, sections, defaultSlug } from "@/docs/manifest"

// Eager raw import keeps lookup synchronous and the whole docs set is small text.
const files = import.meta.glob("../../docs/content/**/*.md", {
  query: "?raw",
  import: "default",
  eager: true,
}) as Record<string, string>

function contentFor(slug: string): string | undefined {
  return files[`../../docs/content/${slug}.md`] ?? files[`../../docs/content/${slug}/index.md`]
}

export default function DocsPage() {
  const params = useParams()
  const slug = params["*"] && params["*"] !== "" ? params["*"].replace(/\/$/, "") : defaultSlug
  const md = contentFor(slug)
  const [dark] = useState(() => document.documentElement.classList.contains("dark"))

  useEffect(() => {
    document.title = `${docsTitle} — ${slug}`
    window.scrollTo(0, 0)
  }, [slug])

  return (
    <div className={dark ? "dark min-h-screen bg-background text-foreground" : "min-h-screen bg-background text-foreground"}>
      <header className="sticky top-0 z-10 flex h-14 items-center gap-3 border-b bg-background/95 px-4 backdrop-blur">
        <Link to="/" className="font-semibold tracking-tight">
          Stratos
        </Link>
        <span className="text-muted-foreground">/</span>
        <Link to="/docs" className="text-sm text-muted-foreground hover:text-foreground">
          Docs
        </Link>
      </header>
      <div className="mx-auto flex w-full max-w-6xl gap-8 px-4 py-6">
        <aside className="hidden w-60 shrink-0 md:block">
          <nav className="sticky top-20 space-y-5 text-sm">
            {sections.map((s) => (
              <div key={s.title}>
                <div className="mb-1.5 font-medium">{s.title}</div>
                <ul className="space-y-0.5 border-l pl-3">
                  {s.pages.map((p) => (
                    <li key={p.slug}>
                      <Link
                        to={`/docs/${p.slug}`}
                        className={
                          slug === p.slug
                            ? "block rounded px-2 py-1 font-medium text-primary"
                            : "block rounded px-2 py-1 text-muted-foreground hover:text-foreground"
                        }
                      >
                        {p.title}
                      </Link>
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </nav>
        </aside>
        <main className="min-w-0 flex-1 pb-16">
          {md ? (
            <ReactMarkdown
              remarkPlugins={[remarkGfm]}
              components={{
                h1: (p) => <h1 className="mb-4 mt-2 text-3xl font-bold tracking-tight" {...p} />,
                h2: (p) => <h2 className="mb-3 mt-8 border-b pb-1.5 text-2xl font-semibold" {...p} />,
                h3: (p) => <h3 className="mb-2 mt-6 text-xl font-semibold" {...p} />,
                p: (p) => <p className="my-3 leading-7" {...p} />,
                ul: (p) => <ul className="my-3 list-disc space-y-1 pl-6" {...p} />,
                ol: (p) => <ol className="my-3 list-decimal space-y-1 pl-6" {...p} />,
                a: (p) => <a className="font-medium text-primary underline underline-offset-2" {...p} />,
                blockquote: (p) => (
                  <blockquote className="my-3 border-l-4 pl-4 text-muted-foreground" {...p} />
                ),
                code: ({ className, children, ...rest }) =>
                  className ? (
                    <code className={`${className} block overflow-x-auto`} {...rest}>
                      {children}
                    </code>
                  ) : (
                    <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-[0.85em]" {...rest}>
                      {children}
                    </code>
                  ),
                pre: (p) => (
                  <pre className="my-4 overflow-x-auto rounded-lg border bg-muted p-4 font-mono text-sm" {...p} />
                ),
                table: (p) => (
                  <div className="my-4 overflow-x-auto">
                    <table className="w-full border-collapse text-sm" {...p} />
                  </div>
                ),
                th: (p) => <th className="border bg-muted px-3 py-2 text-left font-semibold" {...p} />,
                td: (p) => <td className="border px-3 py-2 align-top" {...p} />,
                img: (p) => <img className="my-4 max-w-full rounded-lg border" loading="lazy" {...p} />,
                hr: () => <hr className="my-6" />,
              }}
            >
              {md}
            </ReactMarkdown>
          ) : (
            <div className="py-20 text-center text-muted-foreground">
              Page not found.{" "}
              <Link className="text-primary underline" to="/docs">
                Back to docs
              </Link>
            </div>
          )}
        </main>
      </div>
    </div>
  )
}
