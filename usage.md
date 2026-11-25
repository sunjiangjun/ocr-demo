# Integrating OpenCodeReview in GitHub Actions

This repository reviews pull requests with the official [alibaba/open-code-review](https://github.com/alibaba/open-code-review) Action. The workflow lives in [`.github/workflows/ocr-review.yml`](.github/workflows/ocr-review.yml).

> [!IMPORTANT]
> The workflow must already be on the default branch. `pull_request_target` loads the workflow from the pull request's base branch, so a copy that exists only on a feature branch never runs.

## Contents

- [Integration flow](#integration-flow)
- [Installation](#installation)
- [When it runs](#when-it-runs)
- [Parameters and configuration](#parameters-and-configuration)
- [DeepSeek example](#deepseek-example)
- [Built-in review rules](#built-in-review-rules)

## Integration flow

Follow this order. A pull request opened before the workflow is on the default branch, or before the model settings exist, starts no review.

1. **Add the workflow to the repository.** Place [`.github/workflows/ocr-review.yml`](.github/workflows/ocr-review.yml) in the project. The Action installs its own CLI, so the repository needs no `npm install`. The call site and the full file are in [Installation](#installation).

2. **Commit and push that file to the default branch.** `pull_request_target` loads the workflow from the repository default branch, which is the target branch later pull requests use (`main` in this repository). Push the workflow there before opening a pull request. A copy that exists only on a feature branch never runs.

3. **Configure the LLM.** Under **Settings → Secrets and variables → Actions**, set `OCR_LLM_URL`, `OCR_LLM_AUTH_TOKEN`, `OCR_LLM_MODEL`, and `OCR_LLM_USE_ANTHROPIC`. Names, protocol rules, and the DeepSeek values are in [Parameters and configuration](#parameters-and-configuration) and [DeepSeek example](#deepseek-example).

4. **Open a pull request into that default branch.** Creating the pull request fires `opened` and starts **OpenCodeReview PR Review**. New commits on the pull request branch fire `synchronize` and start another run. Forks are included.

5. **Read the result.** On the pull request, open **Checks / Actions** and select **OpenCodeReview PR Review**. A successful run posts inline comments on **Files changed** and one summary in **Conversation**. A failed run keeps the reason in that run's log and uploaded artifacts.

6. **Ask for another review from the pull request.** A `MEMBER`, `OWNER`, or `COLLABORATOR` can comment:

   ```text
   /open-code-review
   ```

   `@open-code-review` is the other trigger. A new run for the same pull request cancels the one already in progress. Who is allowed to start a run is listed in [When it runs](#when-it-runs).

## Installation

Leave dependency installation to the Action. It installs the CLI inside the job, reads the diff, calls the model, and posts the review on the pull request. Pull request code stays unexecuted.

The job calls the Action like this:

```yaml
- name: Run OpenCodeReview
  uses: alibaba/open-code-review@main
  with:
    llm_url: ${{ secrets.OCR_LLM_URL }}
    llm_auth_token: ${{ secrets.OCR_LLM_AUTH_TOKEN }}
    llm_model: ${{ vars.OCR_LLM_MODEL }}
    llm_use_anthropic: ${{ vars.OCR_LLM_USE_ANTHROPIC }}
    base_ref: ${{ steps.pr-context.outputs.base_ref }}
    head_sha: ${{ steps.pr-context.outputs.head_sha }}
```

`uses: alibaba/open-code-review@main` tracks the latest Action. During the job it runs `npm install -g @alibaba-group/open-code-review@latest` (v1.9.6 or newer).

> [!TIP]
> To freeze behavior, pin both the Action commit SHA and `ocr_version`. Pinning only `@main` still installs whichever CLI release is `latest` at run time.

<details>
<summary>Full workflow</summary>

```yaml
# OpenCodeReview - GitHub Actions PR Auto-Review Demo
#
# Demonstrates invoking the reusable action for both automatic PR review
# (pull_request_target: opened/synchronize/reopened) and on-demand re-review
# via comments starting with '/open-code-review' or '@open-code-review'.
#
# Required secrets/vars (Settings -> Secrets and variables -> Actions):
#   secret   OCR_LLM_URL            LLM API endpoint
#   secret   OCR_LLM_AUTH_TOKEN     LLM auth token (mapped to OCR_LLM_TOKEN)
#   variable OCR_LLM_MODEL          model name
#   variable OCR_LLM_USE_ANTHROPIC  'true' for Anthropic, 'false' for OpenAI-compatible
#
# For the full list of action inputs/outputs and the four comment-posting modes
# (sticky / incremental), see action.yml at the repo root.

name: OpenCodeReview PR Review

# Conditional concurrency group.
#
# GitHub Actions evaluates concurrency BEFORE job-level if-conditions. With a
# flat group (ocr-<pr_number>), every comment on the PR — even an unrelated
# conversation reply that will be skipped — enters the same group and, because
# cancel-in-progress is true, cancels any in-progress review. The result: a
# single normal comment kills a running review, and you see "two runs, one
# cancelled" in the Actions tab.
#
# Fix: matching events (PR events + /open-code-review comments) share a per-PR
# group so a new review cancels any stale one for the same PR. Non-matching
# comments land in a unique noop-<run_id> group that can never collide with a
# real review, so they are skipped instantly without disrupting anything.
concurrency:
  group: >-
    ${{
      (
        github.event_name == 'pull_request_target'
        || (
          github.event_name == 'issue_comment'
          && github.event.issue.pull_request
          && github.event.comment.user.type != 'Bot'
          && (
            github.event.comment.author_association == 'MEMBER'
            || github.event.comment.author_association == 'OWNER'
            || github.event.comment.author_association == 'COLLABORATOR'
          )
          && (
            startsWith(github.event.comment.body, '/open-code-review')
            || startsWith(github.event.comment.body, '@open-code-review')
          )
        )
      )
      && format('ocr-{0}', github.event.pull_request.number || github.event.issue.number)
      || format('noop-{0}', github.run_id)
    }}
  cancel-in-progress: true

on:
  # Use pull_request_target instead of pull_request so that secrets are
  # available even for PRs from forks. This is safe because the reusable
  # action only reads the diff and does not execute any code from the PR.
  pull_request_target:
    types: [opened, synchronize, reopened]
  issue_comment:
    types: [created]

permissions:
  contents: read
  pull-requests: write

jobs:
  code-review:
    runs-on: ubuntu-latest
    timeout-minutes: 30
    # Run on PR events, or on human-authored comments starting with trigger
    # keywords. Bot comments are excluded as a safety net: GITHUB_TOKEN already
    # suppresses events from bot-posted comments, but a PAT/App token would not.
    # issue_comment triggers are further gated on author_association so only
    # MEMBER/OWNER/COLLABORATOR users can spend LLM quota via re-review.
    if: |
      github.event_name == 'pull_request_target'
      || (
        github.event_name == 'issue_comment'
        && github.event.issue.pull_request
        && github.event.comment.user.type != 'Bot'
        && (
          github.event.comment.author_association == 'MEMBER'
          || github.event.comment.author_association == 'OWNER'
          || github.event.comment.author_association == 'COLLABORATOR'
        )
        && (
          startsWith(github.event.comment.body, '/open-code-review')
          || startsWith(github.event.comment.body, '@open-code-review')
        )
      )
    steps:
      - name: Get PR context
        id: pr-context
        if: github.event_name == 'issue_comment'
        uses: actions/github-script@v7
        with:
          script: |
            // For issue_comment events, resolve PR base/head so the action
            // can review the right diff (issue_comment has no top-level
            // pull_request payload fields).
            const prNumber = context.issue.number;
            const { data: pullRequest } = await github.rest.pulls.get({
              owner: context.repo.owner,
              repo: context.repo.repo,
              pull_number: prNumber
            });
            core.setOutput('base_ref', pullRequest.base.ref);
            core.setOutput('head_sha', pullRequest.head.sha);

      - name: Run OpenCodeReview
        uses: alibaba/open-code-review@main
        with:
          llm_url: ${{ secrets.OCR_LLM_URL }}
          llm_auth_token: ${{ secrets.OCR_LLM_AUTH_TOKEN }}
          llm_model: ${{ vars.OCR_LLM_MODEL }}
          llm_use_anthropic: ${{ vars.OCR_LLM_USE_ANTHROPIC }}
          # For issue_comment triggers, pass the resolved refs; for
          # pull_request_target the action resolves them from the event.
          base_ref: ${{ steps.pr-context.outputs.base_ref }}
          head_sha: ${{ steps.pr-context.outputs.head_sha }}
```

</details>

## When it runs

The workflow name in Actions is **OpenCodeReview PR Review**. A new run for the same pull request cancels the one already in progress.

### Automatic review

`pull_request_target` starts a review for pull requests from this repository and from forks:

| Action | When |
|---|---|
| `opened` | A pull request is created |
| `synchronize` | New commits land on the pull request branch |
| `reopened` | A closed pull request is reopened |

### Manual re-review

A comment starts a review when every row below matches:

| Check | Required value |
|---|---|
| Where | A pull request, including its conversation |
| Who | `MEMBER`, `OWNER`, or `COLLABORATOR`, and not a bot |
| Body | Starts with `/open-code-review` or `@open-code-review` |

Issue comments, comments from outside contributors, and any other discussion stay idle.

## Parameters and configuration

Set these four values under **Settings → Secrets and variables → Actions**. They stay out of the workflow file. The Action treats each one as required. A missing name arrives as an empty string: the workflow still starts, `ocr review` fails when it calls the model, and the pull request receives no comments.

| Kind | Name | Action input | Meaning |
|---|---|---|---|
| Secret | `OCR_LLM_URL` | `llm_url` | Full model API URL. OCR requests this URL directly. |
| Secret | `OCR_LLM_AUTH_TOKEN` | `llm_auth_token` | API key. The Action maps it to `OCR_LLM_TOKEN`. |
| Variable | `OCR_LLM_MODEL` | `llm_model` | Model name. It must match a model ID on that endpoint. |
| Variable | `OCR_LLM_USE_ANTHROPIC` | `llm_use_anthropic` | Protocol switch. `true`, `1`, `yes`, or an empty string selects Anthropic. Any other value, including `false`, selects the OpenAI-compatible protocol. |

> [!WARNING]
> An empty `OCR_LLM_USE_ANTHROPIC` selects Anthropic. Set it to `false` for an OpenAI-compatible API.

The URL is the full endpoint path:

| Protocol | URL suffix | `OCR_LLM_USE_ANTHROPIC` |
|---|---|---|
| OpenAI-compatible | `/v1/chat/completions` | `false` |
| Anthropic | `/v1/messages` | `true` |

A bare host such as `https://api.deepseek.com` returns 404. GitHub supplies `GITHUB_TOKEN` automatically. This workflow's `pull-requests: write` permission is what allows the comments.

Display settings stay on the Action defaults:

| Setting | Default | What you see |
|---|---|---|
| Inline comments | on | Findings that map to a line appear on **Files changed** |
| `sticky_summary` | `true` | One summary in **Conversation**, updated in place on later runs |
| `incremental` | `false` | The same finding is posted again as a new inline comment on the next run |
| Artifacts | on | The raw JSON result and stderr are attached to that Actions run |

## DeepSeek example

DeepSeek uses the OpenAI-compatible protocol. Under **Settings → Secrets and variables → Actions**, set:

| Kind | Name | Value |
|---|---|---|
| Secret | `OCR_LLM_URL` | `https://api.deepseek.com/v1/chat/completions` |
| Secret | `OCR_LLM_AUTH_TOKEN` | Your DeepSeek API key |
| Variable | `OCR_LLM_MODEL` | `deepseek-flash` |
| Variable | `OCR_LLM_USE_ANTHROPIC` | `false` |

Create the key on the [DeepSeek platform](https://platform.deepseek.com/api_keys). For a stronger model, set `OCR_LLM_MODEL` to `deepseek-v4-pro`. `deepseek-chat` and `deepseek-reasoner` are retired IDs.

The Action sends `{"thinking": {"type": "disabled"}}` by default, so DeepSeek stays in non-thinking mode. The workflow needs no extra input for that.

After the values are saved and the workflow is on the default branch, open a pull request or comment on an existing one:

```text
/open-code-review
```

Open **Checks / Actions** on that pull request and select **OpenCodeReview PR Review**. A successful run posts inline comments and the summary. A failed run keeps the reason in that run's log and uploaded artifacts. Typical causes are a URL missing the `/v1/chat/completions` suffix, an invalid key, or `OCR_LLM_USE_ANTHROPIC` set to a value other than `false`.

## Built-in review rules

This repository has no `.opencodereview/rule.json`, so each changed file uses the Action's built-in rule. OCR walks the patterns below in order and sends the first match to the model. A supported file that matches none of them uses `default.md` (correctness, security, performance, maintainability, and tests).

| Path in this repository | Rule |
|---|---|
| `*.go` | `go.md` |
| `.github/workflows/*.{yml,yaml}` | `github_workflows.md` |

| Left out of review | Reason |
|---|---|
| `*_test.go` and the other built-in test paths | Test files are excluded by default |
| `.env` and `.env.*`, except `.env.example`, `.env.sample`, and `.env.template` | Secret paths |
| `.ssh`, private keys, `.netrc`, `.npmrc`, `.pypirc`, `.dockercfg` | Secret paths |

### Languages

| Language | Pattern | Rule |
|---|---|---|
| Go | `**/*.go` | `go.md` |
| Java | `**/*.java` | `java.md` |
| Kotlin | `**/*.{kt,kts}` | `kotlin.md` |
| Scala | `**/*.scala` | `scala.md` |
| C# | `**/*.cs` | `csharp.md` |
| F# | `**/*.{fs,fsi,fsx}` | `fsharp.md` |
| JavaScript / TypeScript | `**/*.{ts,js,tsx,jsx,mjs,cjs,mts,cts}` | `ts_js_tsx_jsx.md` |
| Python | `**/*.{py,pyi,ipynb}` | `python.md` |
| Ruby | `**/*.{rb,rake}`, `**/*.gemspec`, `**/{Gemfile,Rakefile}` | `ruby.md` |
| PHP | `**/*.{php,phtml}` | `php.md` |
| Swift | `**/*.swift` | `swift.md` |
| ArkTS (HarmonyOS) | `**/*.ets` | `arkts.md` |
| C | `**/*.c` | `c.md` |
| C++ | `**/*.{cpp,cc,cxx,hpp,hxx}` | `cpp.md` |
| Rust | `**/*.rs` | `rust.md` |
| Zig | `**/*.zig` | `zig.md` |
| CUDA | `**/*.{cu,cuh}` | `cuda_hip.md` |
| HIP | `**/*.hip` | `cuda_hip.md` |
| Verilog / SystemVerilog | `**/*.{v,sv,vh}` | `verilog.md` |
| VHDL | `**/*.{vhd,vhdl}` | `vhdl.md` |
| GLSL, HLSL, WGSL, Metal | `**/*.{glsl,hlsl,wgsl,metal}` | `shader.md` |
| Haskell | `**/*.{hs,lhs}` | `haskell.md` |
| OCaml | `**/*.{ml,mli}` | `ocaml.md` |
| ReasonML | `**/*.{re,rei}` | `ocaml.md` |
| Elm | `**/*.elm` | `elm.md` |
| Clojure / ClojureScript | `**/*.{clj,cljs,cljc,edn}` | `clojure.md` |
| Julia | `**/*.jl` | `julia.md` |
| R | `**/*.R` | `r.md` |
| Nim | `**/*.{nim,nims,nimble}` | `nim.md` |
| MATLAB or Objective-C | `**/*.m` | `matlab.md`, or `objc.md` when the file looks like Objective-C |
| Objective-C++ | `**/*.mm` | `objc.md` |
| PowerShell | `**/*.{ps1,psm1,psd1}` | `powershell.md` |
| Nushell | `**/*.nu` | `nushell.md` |
| Move | `**/*.move` | `move.md` |
| Solidity | `**/*.sol` | `solidity.md` |
| Vyper | `**/*.vy` | `vyper.md` |

### Templates

| Template | Pattern | Rule |
|---|---|---|
| FreeMarker | `**/*.{ftl,ftlh,ftlx}` | `freemarker.md` |
| Handlebars / Mustache | `**/*.{hbs,mustache}` | `handlebars_mustache.md` |
| Jinja | `**/*.{jinja2,j2}` | `jinja.md` |
| Pug | `**/*.pug` | `pug.md` |
| EJS / Liquid / Nunjucks | `**/*.{ejs,liquid,njk}` | `ejs_liquid_njk.md` |
| HAML / Slim | `**/*.{haml,slim}` | `haml_slim.md` |
| ERB | `**/*.erb` | `erb.md` |
| Astro | `**/*.astro` | `astro.md` |

### Configuration, manifests, and schemas

| Kind | Pattern | Rule |
|---|---|---|
| GitHub Actions workflow | `.github/workflows/**/*.{yaml,yml}` | `github_workflows.md` |
| Other GitHub config | `.github/**/*.{yaml,yml}` | `github_config.md` |
| YAML | `**/*.{yaml,yml}` | `yaml.md` |
| JSON / JSON5 / JSONC | `**/*.{json,json5,jsonc}` | `json.md` |
| Java `.properties` | `**/*.properties` | `properties.md` |
| MyBatis mapper / DAO XML | `**/*{mapper,dao}*.xml` | `mapper_dao_xml.md` |
| Maven | `**/pom.xml` | `pom_xml.md` |
| Gradle | `**/build.gradle` | `build_gradle.md` |
| npm | `**/package.json` | `package_json.md` |
| Cargo | `**/Cargo.toml` | `cargo_toml.md` |
| Composer | `**/composer.json` | `composer_json.md` |
| Move package | `**/Move.toml` | `move_toml.md` |
| SQL | `**/*.sql` | `sql.md` |
| GraphQL | `**/*.{graphql,gql}` | `graphql.md` |
| Prisma | `**/*.prisma` | `prisma.md` |
| Protocol Buffers | `**/*.proto` | `protobuf.md` |
| Apache Thrift | `**/*.thrift` | `thrift.md` |
| Cap'n Proto | `**/*.capnp` | `capnp.md` |
| gettext source | `**/*.po` | `po.md` |
| gettext template | `**/*.pot` | `pot.md` |
| Terraform / HCL | `**/*.{tf,tofu,hcl,tfvars}` | `terraform.md` |
| Bicep | `**/*.bicep` | `bicep.md` |
| Jsonnet | `**/*.{jsonnet,libsonnet}` | `jsonnet.md` |
| Dhall | `**/*.dhall` | `dhall.md` |
| Nix | `**/*.nix` | `nix.md` |
| Starlark / Bazel | `**/*.{bzl,bazel,star}`, `**/{BUILD,WORKSPACE}` | `starlark.md` |
| Rego (OPA) | `**/*.rego` | `rego.md` |
