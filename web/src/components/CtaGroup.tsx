import { demoUrl, siteConfig } from "@/lib/site";

type Props = {
  primaryLabel?: string;
  className?: string;
};

export function CtaGroup({
  primaryLabel = "View on GitHub",
  className = "",
}: Props) {
  const status = demoUrl("/v1/status");
  const liveDocs = demoUrl("/docs");

  return (
    <div className={`flex flex-wrap items-center gap-3 ${className}`}>
      <a
        href={siteConfig.githubUrl}
        className="btn-primary inline-flex items-center px-5 py-2.5 text-[0.75rem] font-medium uppercase tracking-[0.12em]"
        rel="noopener noreferrer"
        target="_blank"
      >
        {primaryLabel}
      </a>
      {liveDocs ? (
        <a
          href={liveDocs}
          className="btn-ghost inline-flex items-center px-5 py-2.5 text-[0.75rem] uppercase tracking-[0.12em] text-text"
          rel="noopener noreferrer"
          target="_blank"
        >
          Live demo docs
        </a>
      ) : (
        <a
          href={`${siteConfig.githubUrl}/blob/main/HOW_TO_USE.md`}
          className="btn-ghost inline-flex items-center px-5 py-2.5 text-[0.75rem] uppercase tracking-[0.12em] text-text"
          rel="noopener noreferrer"
          target="_blank"
        >
          Run locally
        </a>
      )}
      {status ? (
        <a
          href={status}
          className="text-[0.7rem] uppercase tracking-[0.12em] text-muted underline-offset-4 hover:text-signal hover:underline"
          rel="noopener noreferrer"
          target="_blank"
        >
          /v1/status
        </a>
      ) : null}
    </div>
  );
}
