# MindEngage LMS

An AI-first Learning Management System by MindEngage with Canvas-like UX, native Generative-AI content pipelines, and full LTI/QTI/xAPI support.

---

## Table of Contents

- [Overview](#overview)  
- [Features](#features)  
- [Architecture](#architecture)  
- [Course Generation Flow](#course-generation-flow)  
- [Exam & Grading Flow](#exam--grading-flow)  
- [Standards & Integrations](#standards--integrations)  
- [Deployment & Extensibility](#deployment--extensibility)  
- [Contributing](#contributing)  

---

## Overview

MindEngage LMS lets your Generative-AI Course‐Factory publish real courses, modules, and quizzes directly into the system. Teachers can review and publish AI‐generated content; students take quizzes and exams with full proctoring hooks; and certification bodies integrate via LTI.

---

## Features

- **AI-Driven Content**: Automated import of courses, lessons, quizzes via REST/GraphQL.  
- **Rich Authoring**: Markdown/HTML editor with IMS Common Cartridge export.  
- **Assessments & Exams**: QTI 3.0 engine, randomized pools, proctoring.  
- **LTI 1.3 Advantage**: Deep-Linking, NRPS, AGS for certification & tool interoperability.  
- **Analytics**: xAPI statements to a Learning Record Store (LRS).  
- **User Management**: OneRoster import, role-based enrollments.  
- **Accessibility**: WCAG 2.2 AA, WAI-ARIA.  
- **Notifications & Calendar**: In-app + email + iCal feeds.  
- **Extensible Plugin SDK**: Webhooks & serverless functions (WASM/Node).

---

## Architecture

```mermaid
graph TD
  A[Web/Mobile Client] -->|REST & GraphQL| B(API Gateway)
  B --> C1[Auth & Identity Service<br/>OIDC, LTI Login]
  B --> C2[Course Service<br/>Content Repo & Versioning]
  B --> C3[Assessment Service<br/>QTI Engine]
  B --> C4[AI Course-Factory Adapter<br/>Webhooks & Event Bus]
  B --> C5[Analytics Service<br/>xAPI LRS]
  B --> C6[Notification Service]
  B --> C7[Certification Adapter<br/>LTI Tool Outbound]

  subgraph Data Stores
    D1[(User DB)]
    D2[(Content Storage<br/>S3/GCS)]
    D3[(Assessment DB)]
    D4[(LRS)]
  end

  C1 --> D1
  C2 --> D2
  C3 --> D3
  C5 --> D4
  C4 -->|events| C2
  C3 -->|grades| C5
  C7 -->|AGS| C3
```

## Course Generation Flow

```mermaid
sequenceDiagram
  participant AI as Generative-AI Engine
  participant CF as Course-Factory Adapter
  participant CS as Course Service
  participant T as Teacher

  AI->>CF: POST /courses (metadata + QTI items)
  CF->>CS: create course, modules, quizzes
  CS-->>CF: 201 Created (courseId)
  CF-->>AI: ACK
  T->>CS: Review & publish course
  CS-->>T: Course live to students
```

## Exam & Grading Flow

```mermaid
sequenceDiagram
  autonumber
  participant Stu as Student Browser
  participant LMS as MindEngage LMS Host
  participant AS as Assessment Service (LTI Tool)
  participant Cert as Certification Platform

  Stu->>LMS: Launch Quiz (LTI)
  LMS->>AS: LTI Launch (id_token + AGS claims)
  AS-->>Stu: Render QTI Player
  Stu->>AS: Submit responses
  AS->>AS: Auto-grade & proctor checks
  AS->>LMS: send scores via LTI AGS
  LMS->>Cert: Deep-Link with score
  Cert-->>Stu: Issue verifiable certificate
```

## Standards & Integrations

| Area                 | Standard / Spec             |
| -------------------- | --------------------------- |
| Course Packaging     | IMS Common Cartridge v1.3   |
| Assessments          | QTI 3.0                     |
| Analytics            | xAPI (Experience API)       |
| Rosters & SIS        | OneRoster v1.2              |
| Tool Launch & Grades | LTI 1.3 Advantage           |
| Authentication       | OAuth 2.1 / OIDC / SAML 2.0 |
| Accessibility        | WCAG 2.2 AA, WAI-ARIA       |

## Deployment & Extensibility

* Infrastructure: Kubernetes + PostgreSQL + Redis + Kafka/NATS + S3/GCS

* CI/CD: GitHub Actions → Helm charts → Canary rollouts

* Plugin SDK: Serverless webhooks (WASM/Node) on content events

* Theming: React + Tailwind UI, CSS variables for branding

## Contributing

1. Fork the repo & create a feature branch.

2. Write Tests for any new behavior.

3. Submit a Pull Request with a clear description.

4. Ensure all CI checks pass (lint, build, tests).