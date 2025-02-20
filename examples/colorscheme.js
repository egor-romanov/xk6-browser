import { check } from 'k6';
import { chromium } from 'k6/x/browser';

export const options = {
  thresholds: {
    checks: ["rate==1.0"]
  }
}

export default async function() {
  const preferredColorScheme = 'dark';

  const browser = chromium.launch({
    headless: __ENV.XK6_HEADLESS ? true : false,
  });

  const context = browser.newContext({
    // valid values are "light", "dark" or "no-preference"
    colorScheme: preferredColorScheme,
  });
  const page = context.newPage();

  try {
    await page.goto(
      'https://googlechromelabs.github.io/dark-mode-toggle/demo/',
      { waitUntil: 'load' },
    )  
    const colorScheme = page.evaluate(() => {
      return {
        isDarkColorScheme: window.matchMedia('(prefers-color-scheme: dark)').matches
      };
    });
    check(colorScheme, {
      'isDarkColorScheme': cs => cs.isDarkColorScheme
    });
  } finally {
    page.close();
    browser.close();
  }
}
