import { Fragment, h } from "./index";

export { Fragment };

export function jsx(
  type: Parameters<typeof h>[0],
  props: Parameters<typeof h>[1],
  key?: string,
) {
  return h(type, key == null ? props : { ...(props ?? {}), key });
}

export const jsxs = jsx;
