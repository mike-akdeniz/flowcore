import type { ReactNode } from "react";
import { Link, NavLink, useLocation } from "react-router-dom";
import {
  AppShell,
  Badge,
  Burger,
  Button,
  Group,
  Menu,
  Text,
  Title,
} from "@mantine/core";
import { useDisclosure } from "@mantine/hooks";
import { api, type Session } from "./api";

// Every item here goes somewhere. There was a disabled "Cases" entry for a page
// that belonged to no slice, which is a promise not kept — a nav item that never
// activates reads as something forgotten rather than something declined.
const nav = [
  { to: "/", label: "My work" },
  { to: "/cases", label: "All work" },
  { to: "/workflows", label: "Workflows" },
];

// A nav item is current when the path is it, or sits beneath it — so the
// workflow detail screen keeps "Workflows" highlighted.
function isCurrent(path: string, to: string) {
  return to === "/" ? path === "/" : path === to || path.startsWith(to + "/");
}

export function Shell({
  session,
  onSignedOut,
  children,
}: {
  session: Session;
  onSignedOut: () => void;
  children: ReactNode;
}) {
  const [opened, { toggle }] = useDisclosure();
  const location = useLocation();
  const person = session.signedInAs!;

  return (
    <AppShell
      header={{ height: 56 }}
      navbar={{
        width: 200,
        breakpoint: "sm",
        collapsed: { mobile: !opened },
      }}
      padding="lg"
    >
      <AppShell.Header>
        <Group h="100%" px="md" justify="space-between">
          <Group gap="sm">
            <Burger opened={opened} onClick={toggle} hiddenFrom="sm" size="sm" />
            <Title order={5}>CaseWork</Title>
          </Group>

          <Group gap="sm">
            {/* Filing lives in the header because it is reachable from anywhere,
                and because it used to be gated on membership of the intake team
                — a gate whose reason was that a draft only appeared in intake's
                queue, so anyone else would file one and lose sight of it. "All
                work" shows every submission, so that reason is gone and with it
                a hardcoded group name that had already drifted. */}
            <Button component={Link} to="/cases/new" size="compact-sm" variant="light">
              New submission
            </Button>

            {/* Grey, not the default primary. Blue is reserved for things you
                can click — a badge that reports the mode is not one of them. */}
            <Badge variant="light" size="sm" color="gray">
              agents: {session.agentMode}
            </Badge>
            {/* Switching identity lives in the account menu, where a real
                application puts it — not beside the page content. */}
            <Menu position="bottom-end">
              <Menu.Target>
                <Button variant="subtle" size="compact-sm">
                  {person.name}
                </Button>
              </Menu.Target>
              <Menu.Dropdown>
                <Menu.Label>Signed in as {person.teams}</Menu.Label>
                <Menu.Divider />
                <Menu.Label>Switch to</Menu.Label>
                {session.roster
                  .filter((member) => member.reference !== person.reference)
                  .map((member) => (
                    <Menu.Item
                      key={member.reference}
                      onClick={async () => {
                        await api.signIn(member.reference);
                        onSignedOut();
                      }}
                    >
                      {member.name}
                      <Text span c="dimmed" size="xs">
                        {" "}
                        · {member.teams}
                      </Text>
                    </Menu.Item>
                  ))}
              </Menu.Dropdown>
            </Menu>

            {/* Outside the menu button on purpose: it is information, not a
                control, and putting it inside would make it look clickable. */}
            {person.teams && (
              <Text size="xs" c="dimmed">
                {person.teams}
              </Text>
            )}
          </Group>
        </Group>
      </AppShell.Header>

      <AppShell.Navbar p="sm">
        {nav.map((item) => (
          <Button
            key={item.to}
            component={NavLink}
            to={item.to}
            variant={isCurrent(location.pathname, item.to) ? "light" : "subtle"}
            justify="flex-start"
            fullWidth
            mb={4}
          >
            {item.label}
          </Button>
        ))}
      </AppShell.Navbar>

      <AppShell.Main>{children}</AppShell.Main>
    </AppShell>
  );
}
