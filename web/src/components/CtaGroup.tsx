import { Button } from "@/components/ui/button";
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
      <Button size="lg" className="uppercase tracking-[0.12em]" asChild>
        <a
          href={siteConfig.githubUrl}
          rel="noopener noreferrer"
          target="_blank"
        >
          {primaryLabel}
        </a>
      </Button>
      {liveDocs ? (
        <Button
          size="lg"
          variant="outline"
          className="uppercase tracking-[0.12em]"
          asChild
        >
          <a href={liveDocs} rel="noopener noreferrer" target="_blank">
            Live demo docs
          </a>
        </Button>
      ) : (
        <Button
          size="lg"
          variant="outline"
          className="uppercase tracking-[0.12em]"
          asChild
        >
          <a
            href={`${siteConfig.githubUrl}/blob/main/HOW_TO_USE.md`}
            rel="noopener noreferrer"
            target="_blank"
          >
            Run locally
          </a>
        </Button>
      )}
      {status ? (
        <Button variant="link" className="h-auto px-0 text-muted-foreground" asChild>
          <a href={status} rel="noopener noreferrer" target="_blank">
            /v1/status
          </a>
        </Button>
      ) : null}
    </div>
  );
}
