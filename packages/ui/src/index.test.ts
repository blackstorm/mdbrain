import { describe, expect, test } from "bun:test";
import { h, raw, renderToString } from "./index";

describe("@mdbrain/ui", () => {
  test("escapes text content and preserves raw HTML blocks", () => {
    const html = renderToString(
      h("div", { class: "shell" }, "Hello <world>", raw("<span>trusted</span>")),
    );

    expect(html).toBe(
      '<div class="shell">Hello &lt;world&gt;<span>trusted</span></div>',
    );
  });

  test("renders function components", () => {
    const Greeting = ({ name }: { name: string }) => h("strong", null, `Hi ${name}`);

    expect(renderToString(h("p", null, h(Greeting, { name: "Mdbrain" })))).toBe(
      "<p><strong>Hi Mdbrain</strong></p>",
    );
  });
});
