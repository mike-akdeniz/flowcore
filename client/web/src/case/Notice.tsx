import type { ReactNode } from "react";
import { Alert, Text } from "@mantine/core";

// A warning or an error, in one look.
//
// The case screen used to say these in three ways: coloured text inside a card,
// red text under a button, and an Alert. One component means a problem looks
// like a problem wherever it appears. Orange is a warning — something the case
// is waiting on a person to fix. Red is an error — something that just failed.
export type Severity = "warning" | "error";

export function Notice({ severity, children }: { severity: Severity; children: ReactNode }) {
  return (
    <Alert color={severity === "error" ? "red" : "orange"} variant="light" p="sm">
      <Text size="sm">{children}</Text>
    </Alert>
  );
}
