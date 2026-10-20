/**
 * Locators based on live DOM research of https://en.ssyoutube.com/ (2026-10-20).
 *
 * Observed structure after submitting a YouTube URL:
 * - form#main-form → input#id_url[name=url] + button#search (label "Download")
 * - results in #main-results / #convert-result
 * - MP4 rows: a#download-mp4-{quality}-audio (with audio) preferred over *-no-audio
 * - Extension upsell popup may appear ("SaveFrom.net Helper") — dismiss, do not install
 * - Alternate path ssyoutube.com/watch?v=… redirects to savefrom.net and may show CAPTCHA —
 *   we do not solve CAPTCHA; worker fails with captcha_required
 */

export const SS_HOME = 'https://en.ssyoutube.com/';

export const locators = {
  urlInput: '#id_url',
  submit: '#search',
  results: '#main-results',
  convertResult: '#convert-result',
  /** Prefer MP4 that includes audio (video+audio track). */
  mp4WithAudio: 'a[id^="download-mp4-"][id$="-audio"]:not([id*="no-audio"])',
  mp4Any: 'a[id^="download-mp4-"]',
  showMore: '#main-results .show-more-button',
  captchaDialog: '.captcha-dialog__document, .captcha-dialog__form, img[alt="Captcha"]',
  extensionClose: 'button.a11y-dialog-close, .extension-popup button',
};

export function youtubeWatchURL(videoID) {
  return `https://www.youtube.com/watch?v=${encodeURIComponent(videoID)}`;
}

export function isValidVideoID(id) {
  return typeof id === 'string' && /^[A-Za-z0-9_-]{6,64}$/.test(id);
}
