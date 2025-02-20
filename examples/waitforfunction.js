import { check } from 'k6';
import { chromium } from 'k6/x/browser';

export const options = {
  thresholds: {
    checks: ["rate==1.0"]
  }
}

export default async function() {
  const browser = chromium.launch({
    headless: true,
  });
  const context = browser.newContext();
  const page = context.newPage();

  try {
    page.evaluate(() => {
      setTimeout(() => {
        const el = document.createElement('h1');
        el.innerHTML = 'Hello';
        document.body.appendChild(el);
      }, 1000);
    });

    const ok = await page.waitForFunction("document.querySelector('h1')", {
      polling: 'mutation',
      timeout: 2000,
    });
    check(ok, { 'waitForFunction successfully resolved': ok.innerHTML() == 'Hello' });
  } finally {
    page.close();
    browser.close();
  }
}
