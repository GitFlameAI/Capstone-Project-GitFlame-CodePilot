# Model Responses

The benchmark runner creates one directory per model:

```text
responses/<model-slug>/
  issue_01_input.md
  issue_01_response.md
  issue_01_metrics.json
```

The `*_input.md` file is the exact issue and repository context sent to the model. The `*_response.md` file is the unedited model answer for manual review.

