# MindEngage LMS Tool

An AI-first Learning Management System by MindEngage with Canvas-like UX, native Generative-AI content pipelines, and full LTI/QTI/xAPI support — now with **dual online/offline modes** for Internet or LAN deployments.

---


## Overview

MindEngage LMS Tool lets your Generative-AI Course‐Factory publish real courses, modules, and quizzes directly into the system.  
It supports **LAN-only deployments** for secure offline classrooms as well as **Internet-connected online deployments** with full LTI and analytics integrations.

Teachers can review and publish AI‐generated content; students take quizzes and exams with full proctoring hooks; and certification bodies integrate via LTI.

---

## Features

- **Dual Mode**: LAN offline (no Internet) or online mode switchable at runtime.  
- **AI-Driven Content**: Automated import of courses, lessons, quizzes via REST/GraphQL.  
- **Rich Authoring**: Markdown/HTML editor with IMS Common Cartridge export.  
- **Assessments & Exams**: QTI 3.0 engine, randomized pools, proctoring.  
- **LTI 1.3 Advantage**: Deep-Linking, NRPS, AGS for certification & tool interoperability.  
- **Analytics**: xAPI statements to a Learning Record Store (LRS) in online mode.  
- **User Management**: Bulk teacher/admin-initiated student onboarding; role-based enrollments.  
- **Offline Auth**: Local users database or JSON with hashed passwords.  
- **Accessibility**: WCAG 2.2 AA, WAI-ARIA.  
- **Notifications & Calendar**: In-app + email + iCal feeds (online mode).  
- **Extensible Plugin SDK**: Webhooks & serverless functions (WASM/Node).

---

## Architecture

### Dual-Mode Overview
```mermaid
graph TD
  subgraph Client
    A[Web/Mobile Client]
  end

  subgraph Backend["MindEngage LMS Gateway"]
    B(API Router)
    C1[Auth & RBAC<br/>Local JWT / OIDC / LTI]
    C2[Course Service<br/>Content Repo & Versioning]
    C3[Exam Service<br/>QTI Engine + Attempts]
    C4[Storage Service<br/>Local FS / S3 / GCS]
    C5[Sync Service<br/>Offline-first Replicator]
  end

  subgraph DataStores
    D1[(User DB<br/>SQLite/Postgres)]
    D2[(Blob Storage)]
    D3[(Exam DB)]
  end

  A -->|REST/GraphQL| B
  B --> C1
  B --> C2
  B --> C3
  B --> C4
  C1 --> D1
  C2 --> D2
  C3 --> D3
  C4 --> D2
  C5 -->|Online only| Cloud[Remote Services<br/>LTI, Analytics, Sync]
```

Modes:

    Offline/LAN → No external network calls, all services run locally, sync disabled.

    Online/Internet → External integrations (LTI, xAPI LRS, cloud storage) enabled.

| Feature          | Offline (LAN)               | Online (Internet)         |
| ---------------- | --------------------------- | ------------------------- |
| Login            | Local JWT from user DB/JSON | OIDC / LTI / Local JWT    |
| Course Import    | Local file upload           | AI Course-Factory webhook |
| Exams            | Fully supported             | Fully supported           |
| Asset Storage    | Local FS                    | S3/GCS/MinIO              |
| Sync             | Disabled                    | Enabled                   |
| Analytics (xAPI) | Local DB (optional)         | Cloud LRS                 |
| User Management  | Bulk CSV/JSON API           | Bulk API + remote roster  |



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
  participant ES as Exam Service
  participant GS as Grading Engine

  Stu->>LMS: Start Attempt
  LMS->>ES: Create attempt
  Stu->>ES: Submit answers
  ES->>GS: Auto-grade (MCQ/Text/Upload OCR)
  GS-->>ES: Scores + feedback
  ES-->>LMS: Store results
```

Offline User Management

Development mode: /auth/login with username=password for rapid testing.

LAN Production mode:

    Use a local SQLite DB or users.json with bcrypt’d passwords.

    Teacher/Admin uploads CSV/JSON roster via POST /users/bulk.

    Server returns credential slips for offline distribution.

    Students can change their own passwords with POST /users/change-password.

Online mode:

    Same bulk import API can be enabled, secured with JWT+RBAC.

    Can force must_change_password on first login.

    Works alongside OIDC/LTI accounts.


Standards & Integrations

| Area                 | Standard / Spec                         |
| -------------------- | --------------------------------------- |
| Course Packaging     | IMS Common Cartridge v1.3               |
| Assessments          | QTI 3.0                                 |
| Analytics            | xAPI (Experience API)                   |
| Rosters & SIS        | OneRoster v1.2                          |
| Tool Launch & Grades | LTI 1.3 Advantage                       |
| Authentication       | OAuth 2.1 / OIDC / SAML 2.0 / Local JWT |
| Accessibility        | WCAG 2.2 AA, WAI-ARIA                   |

## SAT Digital Simulation with Policy + Router
### 1) SAT Policy → Section/Module/Variant model

```mermaid
classDiagram
  class Policy {
    +Navigation.allow_back : bool
    +Navigation.module_locked : bool
    +Sections : Section[]
  }

  class Section {
    +id : string
    +title : string
    +modules : Module[]
  }

  class Module {
    +id : string           // placeholder e.g., "rw-m1", "rw-m2"
    +time_limit_sec : int
    +variants : Variant[]  // only for adaptive M2
    +route : Route         // e.g., by_score threshold
  }

  class Variant {
    +id : string           // e.g., "rw-m2-easy", "rw-m2-hard"
    +time_limit_sec : int? // optional override
  }

  class Route {
    +by_score.threshold : float
    +by_score.lte/lt/gt/gte/default : string
  }

  Policy --> Section
  Section --> Module
  Module --> Variant
  Module --> Route
```


### Item authoring rule

Questions carry question.module_id set to the concrete ID they belong to:

* M1 → rw-m1

* M2-easy → rw-m2-easy

* M2-hard → rw-m2-hard

## Components: where routing happens 

```mermaid
graph TD
  A[Student Browser] -->|attempt APIs| B[API Router]
  B --> C[Exam Service / SQLStore]
  C --> D{RouterForProfile}
  D -->|sat.v1| E[SAT Router]
  D -->|none| F[Sequential Fallback]

  C --> G[Exam DB]
  C --> H[Grader]
  C -->|reads| I[Policy JSON]
  C -->|reads| J[Questions and Keys]

  E --> C
  F --> C

```

## Adaptive attempt lifecycle (SAT two-stage)

```mermaid
sequenceDiagram
  autonumber
  participant S as Student
  participant API as LMS API
  participant ES as Exam Service (SQLStore)
  participant RT as sat.Router
  participant DB as DB

  S->>API: POST /attempts {exam_id,user_id}
  API->>ES: NewAttempt
  ES->>DB: INSERT attempts(..., module_index=0, current_module_id="rw-m1")
  ES-->>API: Attempt{module_index:0, current_module_id:"rw-m1"}

  loop Module 1
    S->>API: SaveResponses / Navigate
    API->>ES: SaveResponses/Navigate
    ES->>DB: UPDATE attempts.responses_json, current_index/max
    ES-->>API: Attempt (RemainingSeconds, etc.)
  end

  S->>API: POST /attempts/{id}/next-module
  API->>ES: AdvanceModule
  ES->>DB: SELECT exam, attempt, responses
  ES->>ES: moduleRawPerf("rw-m1")
  ES->>RT: NextModule(ex, attempt, perf{raw})
  RT-->>ES: "rw-m2-easy" | "rw-m2-hard"
  ES->>DB: UPDATE attempts SET module_index=1, current_module_id=<chosen>, module_started_at/deadline, current_index=firstIdxOf(chosen)
  ES-->>API: Attempt{module_index:1, current_module_id:<chosen>}

  loop Module 2 (chosen variant)
    S->>API: SaveResponses / Navigate
    API->>ES: Enforce module lock/back-forward policy
    ES->>DB: UPDATE attempts...
  end

  S->>API: POST /attempts/{id}/submit
  API->>ES: Submit
  ES->>DB: SELECT questions (with keys)
  ES->>ES: Auto-grade via Grader
  ES->>DB: UPDATE attempts SET status='submitted', score
  ES-->>API: Attempt{status:'submitted', score}
```

## Enforcement & navigation (server-side guards)
```mermaid
flowchart TD
  A[SaveResponses or Navigate] --> B{Attempt submitted}
  B -- yes --> X[409 ErrAttemptSubmitted]
  B -- no --> C{Time over module or overall}
  C -- yes --> Y[409 ErrTimeOver]
  C -- no --> D[Load exam and policy parse nav]
  D --> E{Module locked}
  E -- yes --> F[Select active module id]
  F --> G[Compute window for active module]
  G --> H{Within window}
  H -- no --> Z[409 ErrOutsideModule]
  H -- yes --> I{Allow back}
  E -- no --> I
  I -- no --> J{Target before max reached}
  J -- yes --> K[409 ErrBackwardNavBlocked]
  J -- no --> L{Editing past items}
  L -- yes --> M[409 ErrEditBackBlocked]
  L -- no --> N[Persist responses or index]
  N --> O[200 Attempt]

```

## Attempt state machine (timers + routing)
```mermaid
stateDiagram-v2
  [*] --> InProgress

  state InProgress {
    [*] --> Module1
    Module1 --> RouteDecision: POST /next-module\n(uses moduleRawPerf + Router)
    RouteDecision --> Module2Easy: chosen = "rw-m2-easy"
    RouteDecision --> Module2Hard: chosen = "rw-m2-hard"
    Module2Easy --> ReadyToSubmit
    Module2Hard --> ReadyToSubmit
  }

  InProgress --> Submitted: POST /submit
  Submitted --> [*]

```

## Data model view (fields that make it work)
```mermaid
classDiagram
  class Exam {
    +id: string
    +title: string
    +time_limit_sec: int
    +questions: Question[]
    +profile: string
    +policy: json
  }

  class Question {
    +id: string
    +type: string
    +module_id: string
    +answer_key: string[]?
    +points: float
  }

  class Attempt {
    +id: string
    +exam_id: string
    +user_id: string
    +status: string
    +score: float
    +responses: map
    +module_index: int
    +current_module_id: string
    +module_started_at: int64
    +module_deadline: int64
    +overall_deadline: int64
    +current_index: int
    +max_reached_index: int
  }

  class Router {
    +NextModule(ex, attempt, perf) string
  }

  class SATRouter {
    +NextModule(ex, attempt, perf) string
  }

  Exam "1" --> "*" Question
  Attempt --> Exam
  SATRouter ..|> Router

```

## Authoring-to-runtime alignment
```mermaid
graph TB
  A[Authoring Policy] -->|sections modules variants| B[Exam profile and policy json]
  A2[Authoring Items] -->|module id uses concrete ids| C[Exam questions]

  B --> D[New Attempt]
  C --> D
  D -->|module index 0; current module id rw-m1| R1[Runtime]

  R1 -->|next module| E[Router]
  E -- low raw --> V1[rw-m2-easy]
  E -- high raw --> V2[rw-m2-hard]

  E -->|chosen id| U[Update attempts current_module_id]
  U --> W[Enforcement only chosen variant allowed]

```
