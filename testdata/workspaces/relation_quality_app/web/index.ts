import defaultFormatter, { formatMessage } from "./helpers";
import * as helper from "./helpers";

export function renderPage(enabled: boolean) {
  if (enabled) {
    return helper.normalize(defaultFormatter());
  }
  return formatMessage();
}
