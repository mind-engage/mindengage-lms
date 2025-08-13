import React, { useEffect, useState } from "react";
import {
  AppBar,
  Toolbar,
  Typography,
  Container,
  Box,
  Paper,
  Button,
  Stack,
  Chip,
  Divider,
  CssBaseline,
} from "@mui/material";
import { createTheme, ThemeProvider } from "@mui/material/styles";
import QuizRoundedIcon from "@mui/icons-material/QuizRounded";
import SchoolRoundedIcon from "@mui/icons-material/SchoolRounded";
import AdminPanelSettingsRoundedIcon from "@mui/icons-material/AdminPanelSettingsRounded";
import ArrowForwardRoundedIcon from "@mui/icons-material/ArrowForwardRounded";
import CheckCircleRoundedIcon from "@mui/icons-material/CheckCircleRounded";

const API_BASE = process.env.REACT_APP_API_BASE || "http://localhost:8080/api";

async function api<T>(path: string, opts: RequestInit = {}): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, { cache: "no-store", ...opts });
  if (!res.ok) {
    const t = await res.text().catch(() => "");
    throw new Error(`${res.status} ${res.statusText}${t ? `: ${t}` : ""}`);
  }
  // Handle 204 or empty 200 safely
  const text = await res.text();
  return (text ? JSON.parse(text) : undefined) as T;
}


const theme = createTheme({
  palette: { mode: "light", primary: { main: "#3f51b5" } },
  shape: { borderRadius: 12 },
  components: {
    MuiPaper: { styleOverrides: { root: { borderRadius: 16 } } },
    MuiButton: { defaultProps: { disableRipple: true } },
  },
});

export default function HomeApp() {
  const [status, setStatus] = useState<"online" | "offline" | "checking">("checking");

  useEffect(() => {
    api<void>("/healthz")
      .then(() => setStatus("online"))
      .catch(() => setStatus("offline"));
  }, []);

  return (
    <ThemeProvider theme={theme}>
      <CssBaseline />

      {/* Top App Bar */}
      <AppBar
        position="sticky"
        color="inherit"
        elevation={1}
        sx={{ backdropFilter: "saturate(180%) blur(6px)", background: "rgba(255,255,255,0.9)" }}
      >
        <Toolbar>
          <Box sx={{ width: 10, height: 10, bgcolor: "primary.main", borderRadius: 2, mr: 1.5 }} />
          <Typography variant="h6" sx={{ fontWeight: 600 }}>
            MindEngage • LMS
          </Typography>
          <Box sx={{ flexGrow: 1 }} />
          <Stack direction="row" spacing={1}>
            <Button href="/exam/" variant="text">Exam</Button>
            <Button href="/teacher/" variant="text">Teacher</Button>
            <Button href="/admin/" variant="text">Admin</Button>
          </Stack>
        </Toolbar>
      </AppBar>

      {/* Hero */}
      <Box
        sx={{
          py: { xs: 8, md: 10 },
          background:
            "linear-gradient(180deg, rgba(63,81,181,0.06) 0%, rgba(63,81,181,0.02) 60%, rgba(63,81,181,0) 100%)",
        }}
      >
        <Container maxWidth="lg">
          <Box
            sx={{
              display: "flex",
              flexDirection: { xs: "column", md: "row" },
              alignItems: "stretch",
              gap: 4,
            }}
          >
            {/* Left column */}
            <Box sx={{ flex: { md: "0 0 58%" } }}>
              <Typography variant="h3" fontWeight={800} sx={{ lineHeight: 1.1, fontSize: { xs: 28, sm: 34, md: 40 } }}>
                Welcome to <Box component="span" sx={{ color: "primary.main" }}>MindEngage LMS</Box>
              </Typography>
              <Typography variant="h6" color="text.secondary" sx={{ mt: 2 }}>
                An exam-first LMS that’s fast, teacher-friendly, and student-focused. Create exams, manage classes,
                and deliver assessments with ease — online or on a local network.
              </Typography>

              <Box sx={{ display: "flex", flexDirection: { xs: "column", sm: "row" }, gap: 2, mt: 3 }}>
                <Button
                  size="large"
                  variant="contained"
                  disableElevation
                  endIcon={<ArrowForwardRoundedIcon />}
                  href="/exam/"
                >
                  Take an Exam
                </Button>
                <Button size="large" variant="outlined" href="/teacher/" endIcon={<ArrowForwardRoundedIcon />}>
                  Go to Teacher
                </Button>
                <Button size="large" variant="outlined" href="/admin/" endIcon={<ArrowForwardRoundedIcon />}>
                  Admin Console
                </Button>
              </Box>

              <Stack direction="row" spacing={1.5} sx={{ mt: 3 }} alignItems="center">
                <Chip
                  icon={<CheckCircleRoundedIcon color="success" />}
                  label={
                    status === "checking" ? "Checking system…" : status === "online" ? "System online" : "Offline"
                  }
                  color={status === "online" ? "success" : status === "offline" ? "warning" : "default"}
                  variant="outlined"
                />
                <Typography variant="caption" color="text.secondary">
                  Health from <code>/healthz</code>
                </Typography>
              </Stack>
            </Box>

            {/* Right column */}
            <Box sx={{ flex: { md: "0 0 42%" } }}>
              <Paper elevation={3} sx={{ p: 3, background: "linear-gradient(180deg, #fff, #f7f8ff)" }}>
                <Typography variant="overline" color="text.secondary">
                  At a glance
                </Typography>
                <Stack spacing={1.5} sx={{ mt: 1 }}>
                  <FeatureRow icon={<QuizRoundedIcon />} title="Assessments that flow">
                    Modern, accessible exam experience with autosave, timers, and file uploads.
                  </FeatureRow>
                  <FeatureRow icon={<SchoolRoundedIcon />} title="Teacher-first workflows">
                    Upload exams, manage students, and export/import QTI for portability.
                  </FeatureRow>
                  <FeatureRow icon={<AdminPanelSettingsRoundedIcon />} title="Secure by design">
                    Role-based access, offline mode, and LTI-ready endpoints for LMS integrations.
                  </FeatureRow>
                </Stack>
              </Paper>
            </Box>
          </Box>
        </Container>
      </Box>

      {/* Quick Nav Cards */}
      <Container maxWidth="lg" sx={{ pb: 8 }}>
        <Box
          sx={{
            display: "flex",
            flexWrap: "wrap",
            gap: 3,
            alignItems: "stretch",
          }}
        >
          <Box sx={{ flex: "1 1 280px", minWidth: 0 }}>
            <NavCard
              color="primary.main"
              icon={<QuizRoundedIcon fontSize="large" />}
              title="Exam"
              body="Students: start or resume your assessments."
              href="/exam/"
              cta="Open Exam"
            />
          </Box>
          <Box sx={{ flex: "1 1 280px", minWidth: 0 }}>
            <NavCard
              color="#2E7D32"
              icon={<SchoolRoundedIcon fontSize="large" />}
              title="Teacher"
              body="Create exams, manage classes, and review submissions."
              href="/teacher/"
              cta="Open Teacher"
            />
          </Box>
          <Box sx={{ flex: "1 1 280px", minWidth: 0 }}>
            <NavCard
              color="#6D4C41"
              icon={<AdminPanelSettingsRoundedIcon fontSize="large" />}
              title="Admin"
              body="Manage users, roles, and platform configuration."
              href="/admin/"
              cta="Open Admin"
            />
          </Box>
        </Box>

        <Divider sx={{ my: 5 }} />

        {/* Footer */}
        <Box
          sx={{
            display: "flex",
            flexDirection: { xs: "column", sm: "row" },
            alignItems: "center",
            justifyContent: "space-between",
            gap: 1.5,
          }}
        >
          <Typography variant="body2" color="text.secondary">
            © {new Date().getFullYear()} MindEngage
          </Typography>
          <Box sx={{ display: "flex", gap: 1 }}>
            <Button size="small" href="/teacher/">
              Teachers
            </Button>
            <Button size="small" href="/admin/">
              Admin
            </Button>
            <Button size="small" href="/exam/">
              Students
            </Button>
          </Box>
        </Box>
      </Container>
    </ThemeProvider>
  );
}

/* ===== helpers ===== */

function FeatureRow({
  icon,
  title,
  children,
}: {
  icon: React.ReactNode;
  title: string;
  children: React.ReactNode;
}) {
  return (
    <Box sx={{ display: "flex", gap: 2, alignItems: "flex-start" }}>
      <Box sx={{ mt: 0.5, color: "primary.main" }}>{icon}</Box>
      <Box>
        <Typography fontWeight={600}>{title}</Typography>
        <Typography variant="body2" color="text.secondary">
          {children}
        </Typography>
      </Box>
    </Box>
  );
}

function NavCard({
  color,
  icon,
  title,
  body,
  href,
  cta,
}: {
  color: string;
  icon: React.ReactNode;
  title: string;
  body: string;
  href: string;
  cta: string;
}) {
  return (
    <Paper elevation={1} sx={{ p: 3, height: "100%" }}>
      <Box sx={{ display: "flex", flexDirection: "column", gap: 2, alignItems: "flex-start", height: "100%" }}>
        <Box
          sx={{
            width: 44,
            height: 44,
            borderRadius: 2,
            bgcolor: `${color}15`,
            color,
            display: "grid",
            placeItems: "center",
          }}
        >
          {icon}
        </Box>
        <Box>
          <Typography variant="h6" fontWeight={700}>
            {title}
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
            {body}
          </Typography>
        </Box>
        <Box sx={{ flexGrow: 1 }} />
        <Button href={href} variant="outlined" endIcon={<ArrowForwardRoundedIcon />}>
          {cta}
        </Button>
      </Box>
    </Paper>
  );
}
