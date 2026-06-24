import type { LiveState } from "../../types";

export function LiveSummary({
  live,
}: {
  live: LiveState;
}) {
  return (
    <section className="summary-band">
      <section className="summary-main">
        {live.runID ? (
          <div className="decision-line">
            <span className="chip">Reference {live.runID}</span>
          </div>
        ) : null}
        <h2 className="summary-title">Check a meal plan</h2>
        <p className="summary-text">{live.message || "Enter a meal plan and MealCheck will return a clear decision with supporting details."}</p>
      </section>
    </section>
  );
}

export function SiteFooter() {
  return (
    <footer className="site-footer">
      <a href="/">MealCheck</a>
      <a href="/status.html">Status</a>
      <a href="/about.html">About</a>
      <a href="https://github.com/chranama/MealCheck" target="_blank" rel="noreferrer">
        GitHub
      </a>
    </footer>
  );
}
