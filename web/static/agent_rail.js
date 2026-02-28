(() => {
  function initAgentRail() {
    const host = document.getElementById("agent-avatar-host");
    const chatInput = document.getElementById("agent-chat-input");
    const sendButton = document.getElementById("agent-send-button");
    const subjectLine = document.getElementById("agent-subject-line");
    const typingLine = document.getElementById("agent-typing-line");
    if (!host || !chatInput || !subjectLine || !typingLine) return;
    if (host.dataset.attentionInit === "1") return;
    host.dataset.attentionInit = "1";

    let avatarSvg = null;
    let pupilL = null;
    let pupilR = null;
    let lidL = null;
    let lidR = null;
    const eyeL = { x: 278, y: 540 };
    const eyeR = { x: 605, y: 540 };
    const keyboardFocus = { x: 442, y: 740 };
    const cameraFocus = { x: 442, y: 520 };
    const maxOffset = 20;

    let chatFocused = false;
    let lastMouseMove = 0;
    const mouseWindowMs = 650;
    let mouseSvg = { x: keyboardFocus.x, y: keyboardFocus.y };
    let smoothTarget = { x: keyboardFocus.x, y: keyboardFocus.y };
    let pupilOffset = { x: 0, y: 0 };
    let nextBlinkAt = performance.now() + rand(1200, 4200);
    let blinkPhase = 0;
    let blinkT = 0;

    function rand(a, b) {
      return a + Math.random() * (b - a);
    }
    function clamp(v, lo, hi) {
      return Math.max(lo, Math.min(hi, v));
    }
    function lerp(a, b, t) {
      return a + (b - a) * t;
    }

    function setTyping(typing) {
      typingLine.classList.toggle("agent-typing-paused", !typing);
    }

    function computeTarget() {
      const mouseActive = performance.now() - lastMouseMove < mouseWindowMs;
      if (mouseActive) return mouseSvg;
      return chatFocused ? cameraFocus : keyboardFocus;
    }

    function updateEyes(dt) {
      if (!pupilL || !pupilR) return;
      const target = computeTarget();
      const follow = 0.12;
      smoothTarget.x = lerp(smoothTarget.x, target.x, 1 - Math.pow(1 - follow, dt * 60));
      smoothTarget.y = lerp(smoothTarget.y, target.y, 1 - Math.pow(1 - follow, dt * 60));
      const mid = { x: (eyeL.x + eyeR.x) / 2, y: (eyeL.y + eyeR.y) / 2 };
      let dx = smoothTarget.x - mid.x;
      let dy = smoothTarget.y - mid.y;
      const dist = Math.hypot(dx, dy) || 1;
      const scale = Math.min(1, maxOffset / dist);
      dx *= scale;
      dy *= scale;
      const ease = 0.22;
      pupilOffset.x = lerp(pupilOffset.x, dx, 1 - Math.pow(1 - ease, dt * 60));
      pupilOffset.y = lerp(pupilOffset.y, dy, 1 - Math.pow(1 - ease, dt * 60));
      const transform = `translate(${pupilOffset.x},${pupilOffset.y})`;
      pupilL.setAttribute("transform", transform);
      pupilR.setAttribute("transform", transform);
    }

    function updateBlink(dt) {
      if (!lidL || !lidR) return;
      const now = performance.now();
      if (blinkPhase === 0 && now >= nextBlinkAt) {
        blinkPhase = 1;
        blinkT = 0;
      }
      if (blinkPhase === 0) {
        lidL.setAttribute("height", "0");
        lidR.setAttribute("height", "0");
        return;
      }
      const closeDur = 0.08;
      const openDur = 0.1;
      if (blinkPhase === 1) {
        blinkT += dt;
        const t = clamp(blinkT / closeDur, 0, 1);
        const h = lerp(0, 120, t);
        lidL.setAttribute("height", String(h));
        lidR.setAttribute("height", String(h));
        if (t >= 1) {
          blinkPhase = 2;
          blinkT = 0;
        }
      } else {
        blinkT += dt;
        const t = clamp(blinkT / openDur, 0, 1);
        const h = lerp(120, 0, t);
        lidL.setAttribute("height", String(h));
        lidR.setAttribute("height", String(h));
        if (t >= 1) {
          blinkPhase = 0;
          nextBlinkAt = now + rand(1600, 5200);
        }
      }
    }

    function mapMouseToSvg(event) {
      if (!avatarSvg) return;
      const viewBox = avatarSvg.viewBox && avatarSvg.viewBox.baseVal ? avatarSvg.viewBox.baseVal : null;
      if (!viewBox) return;
      const rect = avatarSvg.getBoundingClientRect();
      if (!rect.width || !rect.height) return;
      const rx = (event.clientX - rect.left) / rect.width;
      const ry = (event.clientY - rect.top) / rect.height;
      mouseSvg.x = viewBox.x + rx * viewBox.width;
      mouseSvg.y = viewBox.y + ry * viewBox.height;
    }

    let lastFrame = performance.now();
    function frame(now) {
      const dt = Math.min(0.033, (now - lastFrame) / 1000);
      lastFrame = now;
      updateEyes(dt);
      updateBlink(dt);
      requestAnimationFrame(frame);
    }

    chatInput.addEventListener("focus", () => {
      chatFocused = true;
      setTyping(false);
    });
    chatInput.addEventListener("blur", () => {
      chatFocused = false;
      setTimeout(() => {
        if (document.activeElement !== chatInput) setTyping(true);
      }, 80);
    });
    chatInput.addEventListener("keydown", (event) => {
      if (event.key === "Enter" && !event.shiftKey) {
        event.preventDefault();
        if (sendButton) sendButton.click();
      }
    });

    if (sendButton) {
      sendButton.addEventListener("click", () => {
        if (!chatInput.value.trim()) return;
        sendButton.classList.add("scale-95");
        setTimeout(() => {
          sendButton.classList.remove("scale-95");
          chatInput.value = "";
          chatInput.blur();
        }, 160);
      });
    }

    window.addEventListener(
      "mousemove",
      (event) => {
        lastMouseMove = performance.now();
        mapMouseToSvg(event);
      },
      { passive: true },
    );

    fetch("/static/cute-chibi.svg?v=eyes-v2")
      .then((response) => response.text())
      .then((svgText) => {
        host.innerHTML = svgText;
        avatarSvg = host.querySelector("svg");
        if (!avatarSvg) return;
        avatarSvg.setAttribute("width", "100%");
        avatarSvg.setAttribute("height", "100%");
        avatarSvg.setAttribute("preserveAspectRatio", "xMidYMid meet");
        pupilL = avatarSvg.querySelector("#pupilL");
        pupilR = avatarSvg.querySelector("#pupilR");
        lidL = avatarSvg.querySelector("#lidL");
        lidR = avatarSvg.querySelector("#lidR");
        setTyping(true);
        requestAnimationFrame(frame);
      })
      .catch(() => {
        setTyping(true);
      });
  }

  window.addEventListener("DOMContentLoaded", initAgentRail);
  window.addEventListener("attention:desk:swap", initAgentRail);
})();

