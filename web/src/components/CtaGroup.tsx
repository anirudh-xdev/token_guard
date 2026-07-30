import { demoUrl, siteConfig } from "@/lib/site";

type Props = {
  primaryLabel?: string;
  className?: string;
};

export function CtaGroup({
  primaryLabel = "View on GitHub",
  className = "",
}: Props) {
  const healthz = demoUrl("/healthz");
  const liveDocs = demoUrl("/docs");

  return (
    <div className={`flex flex-wrap items-center gap-3 ${className}`}>
      <a
        href={siteConfig.githubUrl}
        className="inline-flex items-center bg-signal px-5 py-2.5 text-[0.75rem] font-medium uppercase tracking-[0.12em] text-on-signal transition hover:brightness-110"
        rel="noopener noreferrer"
        target="_blank"
      >
        {primaryLabel}
      </a>
      {liveDocs ? (
        <a
          href={liveDocs}
          className="inline-flex items-center border border-line px-5 py-2.5 text-[0.75rem] uppercase tracking-[0.12em] text-text transition hover:border-signal hover:text-signal"
          rel="noopener noreferrer"
          target="_blank"
        >
          Live demo docs
        </a>
      ) : (
        <a
          href={`${siteConfig.githubUrl}/blob/main/HOW_TO_USE.md`}
          className="inline-flex items-center border border-line px-5 py-2.5 text-[0.75rem] uppercase tracking-[0.12em] text-text transition hover:border-signal hover:text-signal"
          rel="noopener noreferrer"
          target="_blank"
        >
          Run locally
        </a>
      )}
      {healthz ? (
        <a
          href={healthz}
          className="text-[0.7rem] uppercase tracking-[0.12em] text-muted underline-offset-4 hover:text-signal hover:underline"
          rel="noopener noreferrer"
          target="_blank"
        >
          /healthz
        </a>
      ) : null}
    </div>
  );
}
