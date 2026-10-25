;;; mint.el --- Launch Mint-Interfaces apps from Emacs -*- lexical-binding: t; -*-

;; Author: Curtis Tackie
;; Version: 0.2.0
;; Package-Requires: ((emacs "27.1"))
;; Keywords: tools, processes, terminals

;;; Commentary:
;;
;; Runs programs built with Mint-Interfaces (mint-go) from Emacs, with
;; nothing to install beyond Emacs itself.
;;
;; In terminal Emacs the default is "handover": Emacs steps aside, the app
;; runs on the real terminal Emacs is using, and Emacs redraws itself when
;; the app quits.  Nothing is emulated, so the app looks and behaves exactly
;; as it does outside Emacs (colours, mouse, Esc, animation).  Emacs keeps
;; running in the background meanwhile.
;;
;; Emacs also configures each app it launches:
;; - `mint-key-profile' gives every app Emacs keys (C-n/C-p, C-f/C-b, C-g)
;;   or vim keys, through the MINT_KEYS environment variable;
;; - `mint-match-theme' passes the colours of the current Emacs theme
;;   through MINT_PALETTE, so apps match Emacs.
;;
;; Commands:
;;   M-x mint             choose an installed app (C-u: also choose a screen)
;;   M-x mint-run         run a compiled Mint binary
;;   M-x mint-run-source  build a Mint app's Go source if needed, then run it
;;
;; `mint-define-app' defines an `M-x NAME' command for one app.
;;
;; The `term' display mode runs the app inside an Emacs buffer instead, so
;; it can sit beside other windows.  Emacs's built-in `term' is not fully
;; reliable with busy screens (live progress bars); handover is
;; recommended.

;;; Code:

(require 'term)
(require 'json)
(require 'subr-x)

(defgroup mint nil
  "Launch Mint-Interfaces programs from Emacs."
  :group 'tools
  :prefix "mint-")

(defcustom mint-display 'auto
  "How to show a Mint app.
`auto'     handover in terminal Emacs, `term' in graphical Emacs.
`handover' the app takes over the terminal until it quits (terminal Emacs).
`term'     inside a buffer, using Emacs's built-in terminal emulator."
  :type '(choice (const :tag "Automatic" auto)
                 (const :tag "Hand over the terminal" handover)
                 (const :tag "Built-in term buffer" term)))

(defcustom mint-key-profile 'emacs
  "Keys for the apps Emacs launches, passed as MINT_KEYS.
`emacs' adds C-n/C-p to move, C-f/C-b to change, C-g to go back,
C-v/M-v to page.  `vim' adds j/k, h/l, g/G and C-d/C-u.  `default'
uses arrow keys only.  nil leaves MINT_KEYS as it is in the environment.
Arrow keys, Enter, Space, Esc, b and q work in every profile."
  :type '(choice (const emacs) (const vim) (const default)
                 (const :tag "Leave the environment alone" nil)))

(defcustom mint-match-theme t
  "When non-nil, apps use the colours of the current Emacs theme."
  :type 'boolean)

(defcustom mint-theme-faces
  '((accent . (font-lock-keyword-face . :foreground))
    (bright . (font-lock-function-name-face . :foreground))
    (text   . (default . :foreground))
    (muted  . (shadow . :foreground))
    (on     . (success . :foreground))
    (amber  . (warning . :foreground))
    (err    . (error . :foreground))
    (selbg  . (region . :background)))
  "Which Emacs face colour each Mint colour role takes."
  :type '(alist :key-type symbol
                :value-type (cons face (choice (const :foreground) (const :background)))))

(defcustom mint-app-directories nil
  "Folders searched by `mint' for Mint apps (executables answering --mint-info).
Only list folders that hold Mint apps: each executable in them is run
once with --mint-info to read its description."
  :type '(repeat directory))

(defcustom mint-close-buffer-on-exit t
  "In `term' mode, kill the app's buffer when it exits normally."
  :type 'boolean)

(defcustom mint-info-timeout 3
  "Seconds to wait for a program to answer --mint-info."
  :type 'number)

(defcustom mint-before-launch-functions nil
  "Functions called with the app's name just before it starts."
  :type 'hook)

(defcustom mint-after-exit-functions nil
  "Functions called with the app's name and exit status after it quits.
Useful for refreshing buffers the app may have changed."
  :type 'hook)

;;;; Environment passed to apps

(defun mint--hex (colour)
  "Return COLOUR (any Emacs colour name) as #rrggbb, or nil."
  (cond ((not (stringp colour)) nil)
        ((string-match-p "\\`#[0-9a-fA-F]\\{6\\}\\'" colour) (downcase colour))
        ((string-prefix-p "unspecified" colour) nil)
        ((when-let ((rgb (color-values colour)))
           (apply #'format "#%02x%02x%02x" (mapcar (lambda (v) (/ v 256)) rgb))))))

(defun mint--colour-value (colour)
  "Return COLOUR in the form MINT_PALETTE expects for this terminal.
Truecolor displays get #rrggbb; others get the 256-colour index Emacs
itself would use, so the app matches what Emacs shows."
  (when-let ((hex (mint--hex colour)))
    (if (or (display-graphic-p) (>= (display-color-cells) 16777216))
        hex
      (let ((approx (tty-color-approximate (color-values colour))))
        (if approx (number-to-string (nth 1 approx)) hex)))))

(defun mint-palette-spec ()
  "Return the MINT_PALETTE value matching the current Emacs theme."
  (mapconcat
   #'identity
   (delq nil
         (mapcar (lambda (entry)
                   (let* ((role (car entry))
                          (face (cadr entry))
                          (attr (cddr entry))
                          (colour (and (facep face) (face-attribute face attr nil t)))
                          (value (mint--colour-value colour)))
                     (and value (format "%s=%s" role value))))
                 mint-theme-faces))
   ","))

(defun mint--environment ()
  "Environment entries to add when launching an app."
  (delq nil
        (list (and mint-key-profile (format "MINT_KEYS=%s" mint-key-profile))
              (and mint-match-theme
                   (let ((spec (mint-palette-spec)))
                     (and (not (string-empty-p spec)) (concat "MINT_PALETTE=" spec)))))))

;;;; Asking an app about itself

(defun mint--executable (program)
  "Return the full path of PROGRAM, or nil."
  (let ((expanded (expand-file-name program)))
    (cond ((and (file-executable-p expanded) (not (file-directory-p expanded))) expanded)
          ((executable-find program)))))

(defun mint-app-info (program)
  "Return PROGRAM's --mint-info description as an alist, or nil.
Gives up after `mint-info-timeout' seconds, so a program that is not a
Mint app cannot hang Emacs."
  (when-let ((exe (mint--executable program)))
    (let* ((buf (generate-new-buffer " *mint-info*"))
           (proc (make-process :name "mint-info" :buffer buf :noquery t
                               :connection-type 'pipe
                               :command (list exe "--mint-info")
                               :sentinel #'ignore))
           (deadline (+ (float-time) mint-info-timeout)))
      (unwind-protect
          (progn
            (while (and (process-live-p proc) (< (float-time) deadline))
              (accept-process-output proc 0.05))
            (when (and (not (process-live-p proc)) (= 0 (process-exit-status proc)))
              (with-current-buffer buf
                (goto-char (point-min))
                (condition-case nil
                    (let ((json-object-type 'alist) (json-array-type 'list) (json-key-type 'symbol))
                      (json-read))
                  (error nil)))))
        (when (process-live-p proc) (delete-process proc))
        (kill-buffer buf)))))

;;;; Handover: the app runs on the real terminal

(defun mint--tty-device ()
  "Device path of the terminal this frame is on, such as /dev/pts/3.
Emacs calls its first terminal /dev/tty, which means nothing to the
programs it starts, so look up the real device in that case."
  (let ((name (terminal-name)))
    (if (and name (not (equal name "/dev/tty")))
        name
      (or (let ((link (file-symlink-p (format "/proc/%d/fd/0" (emacs-pid)))))
            (and link (string-prefix-p "/dev/" link) link))
          (let ((tty (string-trim (shell-command-to-string
                                   (format "ps -o tty= -p %d" (emacs-pid))))))
            (and (not (member tty '("" "?" "??"))) (concat "/dev/" tty)))))))

(defun mint--finish (name status)
  "Report that app NAME exited with STATUS and run the exit hooks."
  (run-hook-with-args 'mint-after-exit-functions name status)
  (unless (= status 0)
    (message "%s exited with status %d" name status)))

(defun mint--handover (exe args name)
  "Run EXE with ARGS on the real terminal; Emacs resumes when it exits."
  (when (display-graphic-p)
    (user-error "Handover needs terminal Emacs; set `mint-display' to `term' here"))
  (let* ((tty (or (mint--tty-device) (user-error "Cannot find this terminal's device")))
         (terminal (frame-terminal))
         (command (format "exec %s <%s >%s 2>&1"
                          (mapconcat #'shell-quote-argument (cons exe args) " ")
                          (shell-quote-argument tty) (shell-quote-argument tty)))
         (process-environment (append (mint--environment) process-environment))
         (resumed nil)
         (resume (lambda ()
                   (unless resumed
                     (setq resumed t)
                     (ignore-errors (resume-tty terminal))
                     (redraw-display)))))
    (run-hook-with-args 'mint-before-launch-functions name)
    (suspend-tty terminal)
    (condition-case err
        (make-process
         :name name :noquery t :connection-type 'pipe
         :command (list "sh" "-c" command)
         :sentinel (lambda (proc _event)
                     (unless (process-live-p proc)
                       (unwind-protect (funcall resume)
                         (mint--finish name (process-exit-status proc))))))
      ;; Never leave Emacs suspended if the app could not start.
      (error (funcall resume)
             (signal (car err) (cdr err))))))

;;;; Buffer mode: term

(defvar-local mint--pending-bytes ""
  "Bytes of an incomplete UTF-8 character held back from `term'.")

(defun mint--term-filter (proc string)
  "Pass STRING to `term', holding back an incomplete trailing UTF-8 character.
Emacs 29's term mis-decodes a character split between two chunks of
output when it follows an escape sequence, so complete it first."
  (with-current-buffer (process-buffer proc)
    (let* ((s (concat mint--pending-bytes string))
           (n (length s))
           (i (1- n)))
      (while (and (>= i 0) (> i (- n 4)) (= (logand (aref s i) #xC0) #x80))
        (setq i (1- i)))
      (let ((need (if (< i 0) 0
                    (let ((b (aref s i)))
                      (cond ((< b #x80) 1) ((= (logand b #xE0) #xC0) 2)
                            ((= (logand b #xF0) #xE0) 3) ((= (logand b #xF8) #xF0) 4) (t 1))))))
        (if (and (>= i 0) (> (+ i need) n))
            (setq mint--pending-bytes (substring s i)
                  s (substring s 0 i))
          (setq mint--pending-bytes "")))
      ;; Mint always writes UTF-8; term decodes with the locale's coding
      ;; system, which is not UTF-8 when Emacs runs in the C locale.
      (let ((locale-coding-system 'utf-8-unix))
        (term-emulate-terminal proc s)))))

(defun mint--send-escape ()
  "Send Esc to the app at once, rather than waiting for a Meta key."
  (interactive)
  (term-send-raw-string "\e"))

(defun mint--configure-term-buffer (buf)
  "Set up BUF so a full-screen Mint app draws cleanly in `term'."
  (with-current-buffer buf
    (setq-local scroll-margin 0
                hscroll-margin 0
                auto-hscroll-mode nil
                scroll-conservatively 101
                term-scroll-to-bottom-on-output nil
                term-scroll-show-maximum-output nil
                truncate-lines t
                cursor-type nil
                mode-line-format nil)
    (when (bound-and-true-p display-line-numbers-mode) (display-line-numbers-mode -1))
    (when (bound-and-true-p hl-line-mode) (hl-line-mode -1))
    (term-char-mode)
    (let ((map (copy-keymap term-raw-map)))
      (define-key map [escape] #'mint--send-escape)
      (define-key map (kbd "ESC") #'mint--send-escape)
      (define-key map (kbd "C-x") ctl-x-map)
      (use-local-map map))))

(defun mint--run-in-term (exe args name)
  "Run EXE with ARGS in a `term' buffer named after NAME."
  (let* ((process-environment (append (mint--environment) process-environment))
         (buf (apply #'make-term name exe nil args))
         (proc (get-buffer-process buf)))
    (mint--configure-term-buffer buf)
    (when proc
      (set-process-filter proc #'mint--term-filter)
      (set-process-sentinel
       proc (lambda (p event)
              (unless (process-live-p p)
                (let ((status (process-exit-status p)))
                  (if (and mint-close-buffer-on-exit (= status 0) (buffer-live-p buf))
                      (kill-buffer buf)
                    (term-sentinel p event))
                  (mint--finish name status))))))
    (pop-to-buffer-same-window buf)))

;;;; Launching

(defun mint--display ()
  "The display mode to use for this frame."
  (if (eq mint-display 'auto)
      (if (display-graphic-p) 'term 'handover)
    mint-display))

;;;###autoload
(defun mint-run (program &optional name &rest args)
  "Run the Mint binary PROGRAM, named NAME, with ARGS.
How it is shown depends on `mint-display'."
  (interactive (list (read-file-name "Mint app: " nil nil t)))
  (let* ((exe (or (mint--executable program)
                  (user-error "Not an executable program: %s" program)))
         (name (or name (file-name-base exe))))
    (pcase (mint--display)
      ('handover (mint--handover exe args name))
      ('term (mint--run-in-term exe args name))
      (other (user-error "Unknown `mint-display': %S" other)))))

;;;; Building from source

(defun mint--stale-p (binary dir)
  "Non-nil when BINARY is missing or older than a Go file in DIR."
  (or (not (file-exists-p binary))
      (let ((built (file-attribute-modification-time (file-attributes binary))))
        (seq-some (lambda (f)
                    (time-less-p built (file-attribute-modification-time (file-attributes f))))
                  (append (directory-files-recursively dir "\\.go\\'")
                          (directory-files dir t "\\`go\\.\\(mod\\|sum\\)\\'"))))))

(defun mint-build (dir &optional binary)
  "Build the Mint app in DIR into BINARY (default: DIR's name, inside DIR).
Only rebuilds when a Go file is newer than the binary.  Returns BINARY."
  (let* ((dir (file-name-as-directory (expand-file-name dir)))
         (binary (expand-file-name (or binary (file-name-nondirectory (directory-file-name dir))) dir)))
    (when (mint--stale-p binary dir)
      (unless (executable-find "go")
        (user-error "Go is not installed, and %s needs building" dir))
      (message "Building %s…" (file-name-nondirectory binary))
      (let ((default-directory dir)
            (process-environment (cons "CGO_ENABLED=0" process-environment)))
        (with-temp-buffer
          (unless (= 0 (call-process "go" nil t nil "build" "-trimpath" "-ldflags=-s -w" "-o" binary "."))
            (user-error "Build failed in %s:\n%s" dir (string-trim (buffer-string)))))))
    binary))

;;;###autoload
(defun mint-run-source (dir &optional name &rest args)
  "Build the Mint app in DIR if needed, then run it."
  (interactive "DMint app folder: ")
  (apply #'mint-run (mint-build dir) name args))

;;;; Choosing an app

(defvar mint--apps nil
  "Apps registered with `mint-define-app': alist of (NAME . TARGET).")

(defvar mint--info-cache (make-hash-table :test #'equal)
  "Cache of (PATH . MTIME) to --mint-info results.")

(defun mint--cached-info (exe)
  "--mint-info for EXE, cached until the file changes."
  (let* ((key (cons exe (file-attribute-modification-time (file-attributes exe))))
         (hit (gethash key mint--info-cache 'none)))
    (if (not (eq hit 'none))
        hit
      (puthash key (mint-app-info exe) mint--info-cache))))

(defun mint--candidates ()
  "Alist of (NAME . (TARGET . INFO)) for every known app."
  (let (out)
    (dolist (app mint--apps)
      (let* ((target (cdr app))
             (exe (and (not (file-directory-p target)) (mint--executable target))))
        (push (list (car app) target (and exe (mint--cached-info exe))) out)))
    (dolist (dir mint-app-directories)
      (when (file-directory-p dir)
        (dolist (f (directory-files dir t "\\`[^.]"))
          (when (and (file-executable-p f) (not (file-directory-p f)))
            (when-let ((info (mint--cached-info f)))
              (let ((name (or (alist-get 'crumb info) (file-name-nondirectory f))))
                (unless (assoc name out)
                  (push (list name f info) out))))))))
    (nreverse out)))

;;;###autoload
(defun mint (name &optional screen)
  "Open the Mint app NAME, optionally straight at SCREEN.
With a prefix argument, also choose the screen to open."
  (interactive
   (let* ((apps (or (mint--candidates)
                    (user-error "No Mint apps found; use `mint-define-app' or `mint-app-directories'")))
          (annotate (lambda (n)
                      (let ((info (nth 2 (assoc n apps))))
                        (when info
                          (concat "  " (or (alist-get 'note info)
                                           (string-trim (or (alist-get 'title info) ""))))))))
          (name (completing-read "Mint app: "
                                 (lambda (str pred action)
                                   (if (eq action 'metadata)
                                       `(metadata (annotation-function . ,annotate))
                                     (complete-with-action action apps str pred)))
                                 nil t)))
     (list name
           (when current-prefix-arg
             (let ((items (mapcar (lambda (i) (alist-get 'label i))
                                  (alist-get 'items (nth 2 (assoc name apps))))))
               (when items
                 (let ((s (completing-read "Screen: " (cons "Home" items) nil t nil nil "Home")))
                   (unless (equal s "Home") s))))))))
  (let* ((entry (assoc name (mint--candidates)))
         (target (or (nth 1 entry) (user-error "No Mint app named %s" name)))
         (args (and screen (list "--screen" screen))))
    (if (file-directory-p target)
        (apply #'mint-run-source target name args)
      (apply #'mint-run target name args))))

;;;###autoload
(defmacro mint-define-app (command name target &optional docstring)
  "Define `M-x COMMAND' to open the Mint app NAME at TARGET.
TARGET is a compiled binary (path or name on PATH) or the folder of the
app's Go source, which is built when needed.  With a prefix argument,
the command asks which screen to open.  The app is also offered by `mint'."
  `(progn
     (setf (alist-get ,name mint--apps nil nil #'equal) ,target)
     (defun ,command (&optional screen)
       ,(or docstring (format "Open the %s Mint app." name))
       (interactive
        (list (when current-prefix-arg
                (let* ((target ,target)
                       (exe (if (file-directory-p target) (mint-build target) (mint--executable target)))
                       (items (mapcar (lambda (i) (alist-get 'label i))
                                      (alist-get 'items (and exe (mint--cached-info exe))))))
                  (when items
                    (let ((s (completing-read "Screen: " (cons "Home" items) nil t nil nil "Home")))
                      (unless (equal s "Home") s)))))))
       (mint ,name screen))))

(provide 'mint)
;;; mint.el ends here
