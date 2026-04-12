export function normalize(value: string) {
  return value;
}

export function formatMessage() {
  return normalize(" formatted ");
}

export default function defaultFormatter() {
  return formatMessage();
}
