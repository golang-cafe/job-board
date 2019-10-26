(function(window,d,x){
    try {
      if (!window) return;
      var lastSendUrl;
      if (window.navigator.userAgent.search(/(bot|spider|crawl)/ig) > -1) return;
      var post = function(options) {
        var isPushState = options && options.isPushState
        var url = window.location.href;
        if (lastSendUrl === url) return;
        lastSendUrl = url;
        if ('visibilityState' in window.document && window.document.visibilityState === 'prerender') return;
        if ('doNotTrack' in window.navigator && window.navigator.doNotTrack === '1') return;
        var refMatches = window.location.search.match(/[?&](utm_source|source|ref)=([^?&]+)/gi);
        var refs = refMatches ? refMatches.map(function(m) { return m.split('=')[1] }) : [];
        var data = { u: url, x: x };
        if (window.navigator.userAgent) data.a = window.navigator.userAgent;
        if (refs && refs[0]) data.s = refs[0];
        if (window.document.r && !isPushState) data.r = window.document.referrer;
        if (window.innerWidth) data.w = window.innerWidth;
        var request = new XMLHttpRequest();
        request.open('POST', d + '/x', true);
        request.setRequestHeader('Content-Type', 'text/plain; charset=UTF-8');
        request.send(JSON.stringify(data));
      }
      if (window.history && window.history.pushState && Event && window.dispatchEvent) {
        var stateListener = function(type) {
          var orig = window.history[type];
          return function() {
            var rv = orig.apply(this, arguments);
            var event = new Event(type);
            event.arguments = arguments;
            window.dispatchEvent(event);
            return rv;
          };
        };
        window.history.pushState = stateListener('pushState');
        window.addEventListener('pushState', function() {
          post({ isPushState: true });
        });
      }
      post();
    } catch (e) {
        console.log(e);
    }
  })(window, 'https://golang.cafe', '%s');