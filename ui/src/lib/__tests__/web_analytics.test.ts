import { afterEach, describe, expect, it } from "vitest";
import {
  installWebAnalytics,
  shouldInstallWebAnalytics,
} from "../web_analytics";

const BEACON_ELEMENT_ID = "mealcheck-web-analytics";
const SITE_TOKEN = "e0dc84d82bb8404590fd364b3aded26a";

describe("web analytics", () => {
  afterEach(() => {
    document.getElementById(BEACON_ELEMENT_ID)?.remove();
  });

  it("installs the standard Cloudflare beacon on the production hostname", () => {
    const script = installWebAnalytics("mealcheck.dev");

    expect(script).not.toBeNull();
    expect(script).toBe(document.getElementById(BEACON_ELEMENT_ID));
    expect(script).toHaveAttribute("type", "module");
    expect(script).toHaveAttribute(
      "src",
      "https://static.cloudflareinsights.com/beacon.min.js",
    );
    expect(JSON.parse(script?.dataset.cfBeacon ?? "{}")).toEqual({
      token: SITE_TOKEN,
    });
  });

  it("reuses the installed beacon instead of adding a duplicate", () => {
    const first = installWebAnalytics("mealcheck.dev");
    const second = installWebAnalytics("mealcheck.dev");

    expect(second).toBe(first);
    expect(document.querySelectorAll(`#${BEACON_ELEMENT_ID}`)).toHaveLength(1);
  });

  it.each([
    "localhost",
    "127.0.0.1",
    "mealcheck.pages.dev",
    "feature-branch.mealcheck.pages.dev",
    "label-review.mealcheck.dev",
  ])("does not install the beacon on %s", (hostname) => {
    expect(shouldInstallWebAnalytics(hostname)).toBe(false);
    expect(installWebAnalytics(hostname)).toBeNull();
    expect(document.getElementById(BEACON_ELEMENT_ID)).toBeNull();
  });
});
