# Todo

- [ ] Refactor CLI API
  - [ ] Consider use-cases (e.g. simplest use-case should allow accepting just a vod ID and sane defaults for *everything* else)
  - [ ] Consider using a module (e.g. `kingpin`)
- [ ] Logging
  - [ ] File: Move to `$HOME/logs/tvd.log`
    - Exact folder/path may need adjustment
  - [ ] File: Create intermediate directories if they don’t exist
  - [ ] File: (?) Support parameter (CLI flag, config param) to specify location
  - [ ] Contents: Potential for additional events to be logged (spinning up workers, worker receiving a task, chunk download complete, worker completing a task)
- [ ] Config file
  - [ ] File: Default location should be inside folder `$HOME/.config/tvd`
  - [ ] File: Support parameter (CLI flag) to specify location
  - [ ] Content: Should be strictly "config" data, not specific to specific download task
- [ ] Job file
  - [ ] Add support for "job" file which contains config for specific download jobs/tasks
