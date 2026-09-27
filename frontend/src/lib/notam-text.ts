// Company notices may be HTML from a rich-text editor. Render readable text,
// never server-provided markup, scripts, event handlers or remote images.
export function notamText(content: string): string {
  if (!/<\/?[a-z][^>]*>/i.test(content)) return content;
  // Template contents are inert, including resource loads, and never mounted.
  const template = document.createElement("template");
  template.innerHTML = content;
  const fragment = template.content;
  fragment.querySelectorAll("script, style, iframe, object, embed, img, svg, math, template").forEach((node) => node.remove());
  fragment.querySelectorAll("br").forEach((node) => node.replaceWith("\n"));
  fragment.querySelectorAll("p, div, li, h1, h2, h3, h4, h5, h6, tr, blockquote").forEach((node) => node.append("\n"));
  return (fragment.textContent ?? "").replace(/\n{3,}/g, "\n\n").trim();
}
