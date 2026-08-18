import { expect, test } from "@playwright/test";

// End-to-end customer flow: public link → consent → selfie upload → results.
//
// The public search API is mocked at the network boundary so the flow can run
// without a live Go API or indexed data. The web app talks only to
// `/api/v1/public/events/{token}` (GET metadata, POST search), which is exactly
// what a real customer session exercises.
const PUBLIC_TOKEN = "customer-flow-token";

test("customer searches a public Event with a selfie and sees results", async ({ page }) => {
  await page.route("**/api/v1/public/events/**", async (route) => {
    const request = route.request();
    if (request.method() === "GET") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ name: "Sự kiện thử nghiệm", expiresAt: null, downloadsEnabled: false }),
      });
      return;
    }
    if (request.method() === "POST") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ results: [{ photoId: "aaaaaaaa-1111" }, { photoId: "bbbbbbbb-2222" }], nextCursor: null }),
      });
      return;
    }
    await route.continue();
  });

  await page.goto(`/e/${PUBLIC_TOKEN}`);
  await expect(page.getByRole("heading", { name: "Sự kiện thử nghiệm" })).toBeVisible();

  const searchButton = page.getByRole("button", { name: "Tìm ảnh" });
  await page.setInputFiles("#selfie", {
    name: "selfie.jpg",
    mimeType: "image/jpeg",
    buffer: Buffer.from("selfie-bytes"),
  });
  await expect(searchButton).toBeDisabled();

  await page.getByRole("checkbox").check();
  await expect(searchButton).toBeEnabled();

  await searchButton.click();
  await expect(page.getByText("Tìm thấy 2 ảnh")).toBeVisible();
  await expect(page.getByRole("listitem")).toHaveCount(2);
});

const MATCH_ID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa";

test("customer downloads a matched photo when the Event allows downloads", async ({ page }) => {
  await page.route("**/api/v1/public/events/**", async (route) => {
    const request = route.request();
    const url = request.url();
    if (request.method() === "GET") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ name: "Sự kiện có tải ảnh", expiresAt: null, downloadsEnabled: true }),
      });
      return;
    }
    if (request.method() === "POST" && url.endsWith("/downloads")) {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        // A short-lived, object-scoped link. Marked as an attachment so the click
        // downloads rather than navigates, keeping the page state intact.
        body: JSON.stringify({ downloads: [{ photoId: MATCH_ID, url: "data:application/octet-stream;base64,QUJD", expiresAt: "2026-08-15T00:00:00Z" }] }),
      });
      return;
    }
    if (request.method() === "POST") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ results: [{ photoId: MATCH_ID }], nextCursor: null }),
      });
      return;
    }
    await route.continue();
  });

  await page.goto(`/e/${PUBLIC_TOKEN}`);
  await page.setInputFiles("#selfie", { name: "selfie.jpg", mimeType: "image/jpeg", buffer: Buffer.from("selfie-bytes") });
  await page.getByRole("checkbox", { name: /Tôi đồng ý/ }).check();
  await page.getByRole("button", { name: "Tìm ảnh" }).click();
  await expect(page.getByText("Tìm thấy 1 ảnh")).toBeVisible();

  const downloadRequest = page.waitForRequest(
    (request) => request.method() === "POST" && request.url().endsWith("/downloads"),
  );
  await page.getByRole("button", { name: `Tải ảnh ${MATCH_ID}` }).click();
  const request = await downloadRequest;
  expect(request.postDataJSON()).toEqual({ photoIds: [MATCH_ID] });
});
