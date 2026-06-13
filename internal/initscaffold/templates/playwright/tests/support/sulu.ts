// LOCAL Playwright fixture replacing `sulu-playwright-reporter`.
//
// Instead of POSTing results to Sulu's REST API, this attaches Allure labels to the
// allure-results files (which suluctl then uploads). Sulu's AllureImportService reads:
//   - test identity (testId) from the "sulu_id" label,
//   - tags from "tag" labels,
//   - the human display name from the Allure test-case name.
//
// Mechanism: every spec imports `test` from THIS module instead of '@playwright/test'.
// The `{ auto: true }` fixture therefore runs for every test without per-test wiring, reads
// the existing `annotation: { type: 'sulu', description: 'PETSTORE-NNN' }` call sites
// (which stay UNCHANGED), and calls allure-js-commons. The fixture body runs inside the
// test's worker context, so the Allure runtime test case is live when we label it.
import {
  test as base,
  expect,
  type APIResponse,
  type APIRequestContext,
} from '@playwright/test';
import * as allure from 'allure-js-commons';

// Playwright stores tags WITH a leading '@' on testInfo.tags; Sulu tag labels carry the bare name.
function stripAt(tags: readonly string[]): string[] {
  return tags.map((t) => (t.startsWith('@') ? t.slice(1) : t));
}

export const test = base.extend<{ suluAllure: void }>({
  suluAllure: [
    async ({}, use, testInfo) => {
      // testId — Sulu's AllureImportService keys testId off the "sulu_id" label first.
      const sulu = testInfo.annotations.find((a) => a.type === 'sulu');
      if (sulu?.description) {
        await allure.label('sulu_id', sulu.description);
      }
      // Tags — each becomes a "tag" label; Sulu collects all "tag" labels.
      if (testInfo.tags.length > 0) {
        await allure.tags(...stripAt(testInfo.tags));
      }
      // Human display name — overrides the file::title name on the Allure test case.
      await allure.displayName(testInfo.title);

      await use();
    },
    { auto: true },
  ],
});

export { expect };
export type { APIResponse, APIRequestContext };
