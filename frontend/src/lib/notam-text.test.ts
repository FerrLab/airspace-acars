import { expect, it } from "vitest";
import { notamText } from "./notam-text";

it("keeps paragraphs, line breaks and entities readable", () => {
  expect(notamText("<p>One &amp; two</p><p>Three<br>Four</p>")).toBe("One & two\nThree\nFour");
});

it("removes active content and only returns text", () => {
  expect(notamText('<script>alert(1)</script><style>body{}</style><iframe>hidden</iframe><img src=x onerror=alert(1)><a href="javascript:alert(1)">Dispatch</a>')).toBe("Dispatch");
});

it("preserves plain text including line breaks and comparison symbols", () => {
  expect(notamText("Visibility < 1000 m\nContact dispatch")).toBe("Visibility < 1000 m\nContact dispatch");
});
