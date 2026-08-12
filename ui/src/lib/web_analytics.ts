const PRODUCTION_HOSTNAME = "mealcheck.dev";
const BEACON_ELEMENT_ID = "mealcheck-web-analytics";
const BEACON_SOURCE = "https://static.cloudflareinsights.com/beacon.min.js";
const SITE_TOKEN = "e0dc84d82bb8404590fd364b3aded26a";

export function shouldInstallWebAnalytics(hostname: string): boolean {
  return hostname === PRODUCTION_HOSTNAME;
}

export function installWebAnalytics(
  hostname = window.location.hostname,
  documentRef: Document = document,
): HTMLScriptElement | null {
  if (!shouldInstallWebAnalytics(hostname) || !documentRef.head) {
    return null;
  }

  const installed = documentRef.getElementById(BEACON_ELEMENT_ID);
  if (installed instanceof HTMLScriptElement) {
    return installed;
  }

  const script = documentRef.createElement("script");
  script.id = BEACON_ELEMENT_ID;
  script.type = "module";
  script.src = BEACON_SOURCE;
  script.dataset.cfBeacon = JSON.stringify({ token: SITE_TOKEN });
  documentRef.head.appendChild(script);
  return script;
}
