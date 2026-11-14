import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import AIGCMetadata from "./AIGCMetadata";
import AppShell from "./components/AppShell";
import VideoStage from "./components/VideoStage";
import UploadPage from "./components/UploadPage";
import LoginPage from "./components/LoginPage";
import Icon from "./components/Icon";
import { filterVideos, sharedVideoIndex, resolveSharedVideo } from "./discovery";
import ModelRankings from "./ModelRankings";
import { fetchHotFeedPage } from "./hotFeed";
import { createRecommendationState, fetchRecommendationBatch, createReplayBatch, FEED_BUFFER_LIMIT } from "./recommendationContinuation";
import { appendFeedItems, feedTrimCount } from "./feedBuffer";

const TOKEN_KEY = "dreamreel.accessToken";
const USER_KEY = "dreamreel.user";
const PUBLIC_PROFILE_KEY = "dreamreel.publicProfiles";
const FEED_TRANSITION_MS = 320;
const DEFAULT_PLAYBACK_CONFIG = {
  platform: "Web",
  network_type: "DEFAULT",
  preload_count: 3,
  buffer_ms: 1200
};
const FEED_SCENES = [
  { key: "timeline", label: "发现灵感", route: "/timeline", icon: "home" },
  { key: "recommend", label: "为您推荐", route: "/recommend", icon: "auto_awesome" },
  { key: "following", label: "我的关注", route: "/following", icon: "subscriptions" },
  { key: "hot", label: "热门作品", route: "/hotfeed", icon: "local_fire_department" }
];

const image = { currentUser: "/avatar.svg", creator: "/avatar.svg", stage: "/cover.svg", vertical: "/cover.svg", elena: "/avatar.svg", marcus: "/avatar.svg" };

const emptyProfile = {
  id: 0,
  account: "",
  nickname: "",
  avatar_url: image.currentUser,
  bio: "",
  role: "",
  status: 0
};

function App() {
  const [route, setRoute] = useState(() => normalizeRoute(window.location.pathname));
  const [token, setToken] = useState(() => localStorage.getItem(TOKEN_KEY) || "");
  const [user, setUser] = useState(() => readStoredUser());
  const [unreadCount, setUnreadCount] = useState(0);
  const [searchQuery, setSearchQuery] = useState("");
  const [editingProfile, setEditingProfile] = useState(false);
  const onNavigate = useCallback((path) => { setSearchQuery(""); setEditingProfile(false); navigate(path, setRoute); }, []);

  useEffect(() => {
    const handlePopState = () => { setEditingProfile(false); setRoute(normalizeRoute(window.location.pathname)); };
    window.addEventListener("popstate", handlePopState);
    return () => window.removeEventListener("popstate", handlePopState);
  }, []);

  useEffect(() => {
    if (route === "/") {
      navigate(token ? "/recommend" : "/timeline", setRoute);
    }
    if ((route === "/profile" || route === "/me") && !(token && user)) {
      navigate("/timeline", setRoute);
    }
    if (route === "/messages" && !(token && user)) {
      navigate("/auth", setRoute);
    }
  }, [route, token, user]);

  const refreshUnreadCount = useCallback(() => {
    if (!token || !user) {
      setUnreadCount(0);
      return Promise.resolve(0);
    }
    return apiRequest("/api/message-stats/unread", { token })
      .then((data) => {
        const count = Number(data.unread_count || 0);
        setUnreadCount(Number.isFinite(count) ? count : 0);
        return count;
      })
      .catch((error) => {
        if (error.status === 401) {
          setUnreadCount(0);
        }
        return 0;
      });
  }, [token, user]);

  useEffect(() => {
    refreshUnreadCount();
  }, [refreshUnreadCount, route]);

  const session = useMemo(
    () => ({
      token,
      user,
      setAuth(nextToken, nextUser) {
        setToken(nextToken);
        setUser(nextUser);
        if (nextToken) {
          localStorage.setItem(TOKEN_KEY, nextToken);
        } else {
          localStorage.removeItem(TOKEN_KEY);
        }
        localStorage.setItem(USER_KEY, JSON.stringify(nextUser));
      },
      clearAuth() {
        setToken("");
        setUser(null);
        localStorage.removeItem(TOKEN_KEY);
        localStorage.removeItem(USER_KEY);
      }
    }),
    [token, user]
  );

  if (route === "/auth" || route === "/login") return <LoginPage session={session} onNavigate={onNavigate} request={apiRequest}/>;
  const publicUserID = publicUserIDFromRoute(route);
  let page;
  if (route === "/profile" || route === "/me") page = <ProfilePage session={session} onNavigate={onNavigate} editing={editingProfile} setEditing={setEditingProfile}/>;
  else if (publicUserID > 0) page = <PublicProfilePage userID={publicUserID} onNavigate={onNavigate}/>;
  else if (route === "/create" || route === "/upload") page = <UploadPage session={session} onNavigate={onNavigate} request={apiRequest}/>;
  else if (route === "/messages") page = <MessagesPage session={session} onNavigate={onNavigate} onUnreadChange={refreshUnreadCount}/>;
  else if (route === "/models") page = <ModelRankings request={apiRequest} onNavigate={onNavigate}/>;
  else page = <FeedPage feedScene={feedSceneFromRoute(route)} session={session} onNavigate={onNavigate} query={searchQuery} onClearSearch={() => setSearchQuery("")}/>;
  return <AppShell user={user} route={route} authenticated={Boolean(token && user)} unreadCount={unreadCount} onNavigate={onNavigate} onLogout={() => { setEditingProfile(false); logout(session, setRoute); }} onEditProfile={() => setEditingProfile(true)} searchValue={searchQuery} onSearch={value => { setSearchQuery(value); if (!["/timeline", "/recommend", "/following", "/hotfeed"].includes(route)) navigate("/timeline", setRoute); }}>{page}</AppShell>;
}

function FeedPage({ feedScene, session, onNavigate, query, onClearSearch }) {
  const gridMode = Boolean(query.trim());
  const [items, setItems] = useState([]);
  const [index, setIndex] = useState(0);
  const [liked, setLiked] = useState({});
  const [favorited, setFavorited] = useState({});
  const [following, setFollowing] = useState({});
  const [followBusyID, setFollowBusyID] = useState(0);
  const [followError, setFollowError] = useState("");
  const [commentsOpen, setCommentsOpen] = useState(false);
  const [comments, setComments] = useState([]);
  const [commentsState, setCommentsState] = useState("idle");
  const [commentsError, setCommentsError] = useState("");
  const [commentText, setCommentText] = useState("");
  const [feedState, setFeedState] = useState("loading");
  const [contentSource, setContentSource] = useState(feedScene);
  const [feedError, setFeedError] = useState("");
  const [nextCursor, setNextCursor] = useState("");
  const [hasMore, setHasMore] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [playbackConfig, setPlaybackConfig] = useState(DEFAULT_PLAYBACK_CONFIG);
  const [feedRequestID, setFeedRequestID] = useState("");
  const [swipe, setSwipe] = useState(null);
  const wheelLocked = useRef(false);
  const transitionTimer = useRef(null);
  const feedMainRef = useRef(null);
  const dragRef = useRef(null);
  const swipeRef = useRef(null);
  const loadingMoreRef = useRef(false);
  const feedRequestIDRef = useRef("");
  const pagingSourceRef = useRef(feedScene);
  const exposedRef = useRef(new Set());
  const preloadedVideoRef = useRef(new Set());
  const recommendationRef = useRef(createRecommendationState());
  const continuationAbortRef = useRef(null);
  const prefetchedEntryRef = useRef("");
  const itemsRef = useRef(items);
  itemsRef.current = items;
  const currentFeedScene = FEED_SCENES.find((scene) => scene.key === feedScene) || FEED_SCENES[0];

  const loadFeed = useCallback(() => {
    let live = true;
    continuationAbortRef.current?.abort();
    feedRequestIDRef.current = "";
    prefetchedEntryRef.current = "";
    pagingSourceRef.current = feedScene;
    setContentSource(feedScene);
    if (requiresAuthFeed(feedScene) && !session.token) {
      setSwipe(null);
      setItems([]);
      setIndex(0);
      setCommentsOpen(false);
      setFeedError("");
      setNextCursor("");
      setHasMore(false);
      setLoadingMore(false);
      setFeedState("auth");
      return () => {
        live = false;
      };
    }

    const requestID = createFeedRequestID(feedScene);
    feedRequestIDRef.current = requestID;
    exposedRef.current = new Set();
    loadingMoreRef.current = false;
    setFeedRequestID(requestID);
    setNextCursor("");
    setHasMore(false);
    setLoadingMore(false);
    setFeedState("loading");
    setFeedError("");
    const controller = new AbortController();
    continuationAbortRef.current = controller;
    const timeout = window.setTimeout(() => controller.abort(), 7000);
    const initial = feedScene === "recommend"
      ? fetchRecommendationBatch(createRecommendationState(), {
          fetchPage: (scene, cursor, id) => fetchFeedPage(scene, session.token, cursor, id, controller.signal),
          seenIDs: new Set(),
          createRequestID: createFeedRequestID
        }).then((batch) => {
          if (!batch.items.length && batch.error) throw batch.error;
          const progress = batch.state[batch.scene];
          return { items: batch.items, next_cursor: progress.cursor, has_more: progress.hasMore, source: batch.scene, requestID: batch.requestID, continuation: batch.state };
        })
      : feedScene === "hot"
        ? fetchHotFeedPage({
            fetchPage: (scene, cursor) => fetchFeedPage(scene, session.token, cursor, requestID, controller.signal)
          })
        : fetchFeedPage(feedScene, session.token, "", requestID, controller.signal);
    initial
      .then(data => resolveSharedVideo(data, window.location.search, apiRequest, controller.signal))
      .then((data) => {
        if (!live || feedRequestIDRef.current !== requestID) return;
        if (data.continuation) recommendationRef.current = data.continuation;
        const actualSource = data.source || feedScene;
        pagingSourceRef.current = actualSource;
        setContentSource(actualSource);
        const nextItems = (data.items || []).map((item) => mapFeedItem(item, actualSource, data.requestID || requestID));
        setSwipe(null);
        setItems(nextItems);
        setLiked(viewerActionMap(nextItems, "liked"));
        setFavorited(viewerActionMap(nextItems, "favorited"));
        setIndex(Math.max(0, sharedVideoIndex(nextItems, window.location.search)));
        setCommentsOpen(false);
        setNextCursor(data.next_cursor || "");
        setHasMore(Boolean(data.has_more && data.next_cursor));
        setFeedState("ready");
        if (data.shareError) setFeedError(data.shareError);
      })
      .catch((error) => {
        if (!live || feedRequestIDRef.current !== requestID) return;
        if (error.status === 401 && requiresAuthFeed(feedScene)) {
          setItems([]);
          setIndex(0);
          setCommentsOpen(false);
          setNextCursor("");
          setHasMore(false);
          setFeedState("auth");
          return;
        }
        setItems([]);
        setFeedError(error.message || `${currentFeedScene.label}加载失败`);
        setFeedState("error");
      })
      .finally(() => window.clearTimeout(timeout));
    return () => {
      live = false;
      window.clearTimeout(timeout);
      continuationAbortRef.current?.abort();
      if (feedRequestIDRef.current === requestID) feedRequestIDRef.current = "";
    };
  }, [currentFeedScene.label, feedScene, session.token]);

  const continueRecommendation = useCallback(async () => {
    if (loadingMoreRef.current || feedState !== "ready" || !session.token) return;
    const generation = feedRequestIDRef.current;
    const controller = new AbortController();
    continuationAbortRef.current = controller;
    const timeout = window.setTimeout(() => controller.abort(), 7000);
    loadingMoreRef.current = true;
    setLoadingMore(true);
    try {
      const batch = await fetchRecommendationBatch(recommendationRef.current, {
        fetchPage: (scene, cursor, id) => fetchFeedPage(scene, session.token, cursor, id, controller.signal),
        seenIDs: new Set(itemsRef.current.map((item) => item.video_id)),
        createRequestID: createFeedRequestID
      });
      if (feedRequestIDRef.current !== generation) return;
      recommendationRef.current = batch.state;
      const progress = batch.state[batch.scene];
      setNextCursor(progress.cursor);
      setHasMore(progress.hasMore);
      if (batch.items.length) {
        const added = batch.items.map((item) => mapFeedItem(item, batch.scene, batch.requestID));
        setItems((state) => appendFeedItems(state, added));
        mergeViewerActions(added, setLiked, "liked");
        mergeViewerActions(added, setFavorited, "favorited");
      } else {
        const replayID = createFeedRequestID("replay");
        setItems((state) => [...state, ...createReplayBatch(state, replayID)]);
      }
    } catch (error) {
      if (feedRequestIDRef.current !== generation) return;
      if (error.status === 401) {
        session.clearAuth();
        onNavigate("/auth");
      }
    } finally {
      window.clearTimeout(timeout);
      if (feedRequestIDRef.current === generation) {
        loadingMoreRef.current = false;
        setLoadingMore(false);
      }
    }
  }, [feedState, onNavigate, session]);

  const loadMoreFeed = useCallback(() => {
    if (feedScene === "recommend") {
      void continueRecommendation();
      return;
    }
    if (loadingMoreRef.current || feedState !== "ready" || !hasMore || !nextCursor || requiresAuthFeed(feedScene) && !session.token) {
      return;
    }

    const requestID = feedRequestIDRef.current || feedRequestID || createFeedRequestID(feedScene);
    loadingMoreRef.current = true;
    setLoadingMore(true);
    const pending = feedScene === "hot"
      ? fetchHotFeedPage({
          fetchPage: (scene, cursor) => fetchFeedPage(scene, session.token, cursor, requestID),
          cursor: nextCursor,
          source: pagingSourceRef.current
        })
      : fetchFeedPage(feedScene, session.token, nextCursor, requestID);
    pending.then((data) => {
        if (feedRequestIDRef.current !== requestID) return;
        const actualSource = data.source || feedScene;
        pagingSourceRef.current = actualSource;
        setContentSource(actualSource);
        const nextItems = (data.items || []).map((item) => mapFeedItem(item, actualSource, requestID));
        setItems((state) => appendFeedItems(state, nextItems));
        mergeViewerActions(nextItems, setLiked, "liked");
        mergeViewerActions(nextItems, setFavorited, "favorited");
        setNextCursor(data.next_cursor || "");
        setHasMore(Boolean(data.has_more && data.next_cursor));
      })
      .catch((error) => {
        if (feedRequestIDRef.current !== requestID) return;
        if (error.status === 401 && requiresAuthFeed(feedScene)) {
          session.clearAuth();
          onNavigate("/auth");
        }
      })
      .finally(() => {
        if (feedRequestIDRef.current !== requestID) return;
        loadingMoreRef.current = false;
        setLoadingMore(false);
      });
  }, [continueRecommendation, feedRequestID, feedScene, feedState, hasMore, nextCursor, onNavigate, session]);

  useEffect(() => {
    if (!session.token) {
      setFollowing({});
      return undefined;
    }

    let live = true;
    loadFollowingMap(session.token)
      .then((map) => {
        if (live) {
          setFollowing(map);
        }
      })
      .catch((error) => {
        if (error.status === 401) {
          session.clearAuth();
          onNavigate("/auth");
        }
      });
    return () => {
      live = false;
    };
  }, [onNavigate, session]);

  useEffect(() => {
    return loadFeed();
  }, [loadFeed]);

  useEffect(() => {
    if (!session.token) {
      setPlaybackConfig(DEFAULT_PLAYBACK_CONFIG);
      preloadedVideoRef.current = new Set();
      return undefined;
    }

    let live = true;
    fetchPlaybackConfig(session.token)
      .then((config) => {
        if (live) {
          setPlaybackConfig(normalizePlaybackConfig(config));
        }
      })
      .catch((error) => {
        if (error.status === 401) {
          session.clearAuth();
          onNavigate("/auth");
          return;
        }
        if (live) {
          setPlaybackConfig(DEFAULT_PLAYBACK_CONFIG);
        }
      });
    return () => {
      live = false;
    };
  }, [onNavigate, session.clearAuth, session.token]);

  useEffect(() => {
    if (feedScene !== "recommend" && feedState === "ready" && items.length > 0 && index >= items.length - 3) {
      loadMoreFeed();
    }
  }, [feedScene, feedState, index, items.length, loadMoreFeed]);

  useEffect(() => {
    if (feedScene !== "recommend" || feedState !== "ready" || loadingMore || swipe || !items.length || index < items.length - 3) return;
    const entry = items[index]?.entry_id;
    // 同一播放位置只补充一次，少量视频复播时也不会连续触发请求或追加。
    if (!entry || prefetchedEntryRef.current === entry) return;
    prefetchedEntryRef.current = entry;
    void continueRecommendation();
  }, [continueRecommendation, feedScene, feedState, index, items, loadingMore, swipe]);

  useEffect(() => {
    if (feedScene !== "recommend" || swipe) return;
    const remove = feedTrimCount(items.length, index);
    if (!remove) return;
    const retained = new Set(items.slice(remove).map((item) => String(item.video_id)));
    // 回收较早的播放记录，保留当前视频及少量回看位置。
    setItems((state) => state.slice(remove));
    setIndex((value) => value - remove);
    setLiked((state) => Object.fromEntries(Object.entries(state).filter(([id]) => retained.has(id))));
    setFavorited((state) => Object.fromEntries(Object.entries(state).filter(([id]) => retained.has(id))));
  }, [feedScene, index, items, swipe]);

  useEffect(() => {
    return () => {
      if (transitionTimer.current) {
        window.clearTimeout(transitionTimer.current);
      }
    };
  }, []);

  useEffect(() => {
    swipeRef.current = swipe;
  }, [swipe]);

  useEffect(() => {
    if (!swipe && index >= items.length && items.length > 0) {
      setIndex(items.length - 1);
    }
  }, [index, items.length, swipe]);

  const current = gridMode ? undefined : items[index];
  const visibleCurrent = gridMode ? undefined : swipe ? items[swipe.fromIndex] : current;
  const visibleNext = swipe ? items[swipe.toIndex] : null;
  const trackStyle = getFeedTrackStyle(swipe);

  const updateCurrentItem = useCallback((videoID, patch) => {
    setItems((state) => state.map((item) => (item.video_id === videoID ? { ...item, ...patch } : item)));
  }, []);

  useEffect(() => {
    if (!current || feedState !== "ready" || !session.token) return;
    const scene = current.feed_scene || feedScene;
    const requestID = current.request_id || feedRequestID;
    const exposureKey = `${scene}:${requestID}:${current.video_id}`;
    if (!requestID || exposedRef.current.has(exposureKey)) return;
    exposedRef.current.add(exposureKey);
    if (exposedRef.current.size > FEED_BUFFER_LIMIT * 2) {
      exposedRef.current = new Set([...exposedRef.current].slice(-FEED_BUFFER_LIMIT));
    }
    apiRequest("/api/video-view-events", {
      method: "POST",
      token: session.token,
      body: {
        video_id: current.video_id,
        scene,
        request_id: requestID,
        event_type: "exposed",
        watch_ms: 0,
        completed: false
      }
    }).catch((error) => {
      if (error.status === 401 && requiresAuthFeed(feedScene)) {
        session.clearAuth();
        onNavigate("/auth");
      }
    });
  }, [current, feedRequestID, feedScene, feedState, onNavigate, session]);

  useEffect(() => {
    if (!current || feedState !== "ready" || !session.token) return undefined;
    let live = true;
    fetchPreloadVideos(session.token, current.video_id, playbackConfig.preload_count)
      .then((data) => {
        if (!live) return;
        prewarmVideoAssets(data.items || [], preloadedVideoRef.current);
      })
      .catch((error) => {
        if (error.status === 401 && requiresAuthFeed(feedScene)) {
          session.clearAuth();
          onNavigate("/auth");
        }
      });
    return () => {
      live = false;
    };
  }, [current?.video_id, feedScene, feedState, onNavigate, playbackConfig.preload_count, session.clearAuth, session.token]);

  const reportPlaybackQoS = useCallback(
    (item, metrics) => {
      if (!session.token || !item?.video_id) return;
      const payload = buildPlaybackQoSPayload(metrics);
      if (!payload) return;
      apiRequest("/api/playback-qos-reports", {
        method: "POST",
        token: session.token,
        headers: {
          "Idempotency-Key": createPlaybackQoSKey(item, metrics)
        },
        body: {
          video_id: item.video_id,
          ...payload
        }
      }).catch((error) => {
        if (error.status === 401) {
          session.clearAuth();
          onNavigate("/auth");
        }
      });
    },
    [onNavigate, session.clearAuth, session.token]
  );

  const requireLogin = useCallback(() => {
    if (session.token) return true;
    onNavigate("/auth");
    return false;
  }, [onNavigate, session.token]);

  const setLike = useCallback(async () => {
    if (!current || swipe || !requireLogin()) return;
    try {
      const nextLiked = !Boolean(liked[current.video_id]);
      const data = await apiRequest(`/api/videos/${current.video_id}/like`, {
        method: nextLiked ? "PUT" : "DELETE",
        token: session.token,
        headers: {
          "Idempotency-Key": `web-like-${current.video_id}-${Date.now()}`
        }
      });
      setLiked((state) => ({ ...state, [current.video_id]: Boolean(data.active) }));
      updateCurrentItem(current.video_id, { like_count: data.like_count ?? current.like_count });
    } catch (error) {
      if (error.status === 401) {
        session.clearAuth();
        onNavigate("/auth");
      }
    }
  }, [current, liked, onNavigate, requireLogin, session, swipe, updateCurrentItem]);

  const setFavorite = useCallback(async () => {
    if (!current || swipe || !requireLogin()) return;
    try {
      const nextFavorited = !Boolean(favorited[current.video_id]);
      const data = await apiRequest(`/api/videos/${current.video_id}/favorite`, {
        method: nextFavorited ? "PUT" : "DELETE",
        token: session.token,
        headers: {
          "Idempotency-Key": `web-favorite-${current.video_id}-${Date.now()}`
        }
      });
      setFavorited((state) => ({ ...state, [current.video_id]: Boolean(data.active) }));
      updateCurrentItem(current.video_id, { favorite_count: data.favorite_count ?? current.favorite_count });
    } catch (error) {
      if (error.status === 401) {
        session.clearAuth();
        onNavigate("/auth");
      }
    }
  }, [current, favorited, onNavigate, requireLogin, session, swipe, updateCurrentItem]);

  const setFollow = useCallback(async () => {
    if (!current || swipe || !requireLogin()) return;
    if (current.author_id === session.user?.id) return;

    const authorID = current.author_id;
    const nextFollowing = !Boolean(following[authorID]);
    setFollowBusyID(authorID);
    setFollowError("");
    try {
      const data = await apiRequest(`/api/users/me/following/${authorID}`, {
        method: nextFollowing ? "PUT" : "DELETE",
        token: session.token,
        headers: {
          "Idempotency-Key": `web-follow-${authorID}-${Date.now()}`
        }
      });
      setFollowing((state) => ({ ...state, [authorID]: Boolean(data.following) }));
      updateSessionRelationCount(session, data.following_count);
    } catch (error) {
      if (error.status === 401) {
        session.clearAuth();
        onNavigate("/auth");
        return;
      }
      setFollowError(error.message || "关注操作失败");
    } finally {
      setFollowBusyID(0);
    }
  }, [current, following, onNavigate, requireLogin, session, swipe]);

  const loadComments = useCallback(() => {
    if (!current) return undefined;
    let live = true;
    setCommentsState("loading");
    setCommentsError("");
    apiRequest(`/api/videos/${current.video_id}/comments?limit=50`)
      .then((data) => {
        if (!live) return;
        const nextComments = data.items || [];
        setComments(nextComments);
        if (!data.has_more && nextComments.length > Number(current.comment_count || 0)) {
          updateCurrentItem(current.video_id, { comment_count: nextComments.length });
        }
        setCommentsState("ready");
      })
      .catch((error) => {
        if (!live) return;
        setComments([]);
        setCommentsError(error.message || "评论加载失败");
        setCommentsState("error");
      });
    return () => {
      live = false;
    };
  }, [current]);

  useEffect(() => {
    if (!commentsOpen) return undefined;
    return loadComments();
  }, [commentsOpen, loadComments]);

  async function submitComment() {
    if (!current || !requireLogin()) return;
    const content = commentText.trim();
    if (!content) return;
    try {
      const data = await apiRequest(`/api/videos/${current.video_id}/comments`, {
        method: "POST",
        token: session.token,
        headers: {
          "Idempotency-Key": `web-comment-${current.video_id}-${Date.now()}`
        },
        body: {
          content
        }
      });
      setCommentText("");
      setComments((state) => [data, ...state.filter((item) => item.id !== data.id)]);
      updateCurrentItem(current.video_id, { comment_count: data.comment_count ?? current.comment_count + 1 });
      setCommentsState("ready");
    } catch (error) {
      if (error.status === 401) {
        session.clearAuth();
        onNavigate("/auth");
        return;
      }
      setCommentsError(error.message || "评论发布失败");
      setCommentsState("error");
    }
  }

  function getStageHeight() {
    return feedMainRef.current?.clientHeight || window.innerHeight || 1;
  }

  function moveTo(nextIndex) {
    if (swipe || nextIndex === index || nextIndex < 0 || nextIndex >= items.length) return;
    const direction = nextIndex > index ? "next" : "prev";
    const height = getStageHeight();
    if (transitionTimer.current) {
      window.clearTimeout(transitionTimer.current);
    }
    setSwipe({
      fromIndex: index,
      toIndex: nextIndex,
      direction,
      height,
      offset: 0,
      settling: ""
    });
    window.requestAnimationFrame(() => {
      setSwipe((state) =>
        state && state.fromIndex === index && state.toIndex === nextIndex
          ? {
              ...state,
              offset: direction === "next" ? -height : height,
              settling: "commit"
            }
          : state
      );
    });
    transitionTimer.current = window.setTimeout(() => {
      setIndex(nextIndex);
      setSwipe(null);
      transitionTimer.current = null;
    }, FEED_TRANSITION_MS);
  }

  function settleSwipe(commit) {
    const active = swipeRef.current;
    if (!active) return;
    if (transitionTimer.current) {
      window.clearTimeout(transitionTimer.current);
    }
    setSwipe({
      ...active,
      offset: commit ? (active.direction === "next" ? -active.height : active.height) : 0,
      settling: commit ? "commit" : "cancel"
    });
    transitionTimer.current = window.setTimeout(() => {
      if (commit) {
        setIndex(active.toIndex);
      }
      setSwipe(null);
      transitionTimer.current = null;
    }, FEED_TRANSITION_MS);
  }

  function handlePointerDown(event) {
    if (event.button > 0 || swipe || items.length < 2 || isInteractiveTarget(event.target) || event.target?.closest?.('.dr-player-info')) return;
    dragRef.current = {
      pointerId: event.pointerId,
      startY: event.clientY,
      fromIndex: index,
      active: false,
      direction: "",
      height: 0
    };

  }

  function handlePointerMove(event) {
    const drag = dragRef.current;
    if (!drag || drag.pointerId !== event.pointerId) return;
    const delta = event.clientY - drag.startY;
    if (!drag.active) {
      if (Math.abs(delta) < 8) return;
      const direction = delta < 0 ? "next" : "prev";
      const toIndex = direction === "next" ? drag.fromIndex + 1 : drag.fromIndex - 1;
      if (toIndex < 0 || toIndex >= items.length) {
        return;
      }
      const height = getStageHeight();
      event.currentTarget.setPointerCapture(event.pointerId);
      dragRef.current = {
        ...drag,
        active: true,
        direction,
        toIndex,
        height
      };
      setSwipe({
        fromIndex: drag.fromIndex,
        toIndex,
        direction,
        height,
        offset: clampSwipeOffset(direction, delta, height),
        settling: ""
      });
      event.preventDefault();
      return;
    }

    setSwipe((state) =>
      state
        ? {
            ...state,
            offset: clampSwipeOffset(state.direction, delta, state.height),
            settling: ""
          }
        : state
    );
    event.preventDefault();
  }

  function handlePointerEnd(event) {
    const drag = dragRef.current;
    if (!drag || drag.pointerId !== event.pointerId) return;
    dragRef.current = null;
    const active = swipeRef.current;
    if (!drag.active || !active) return;
    const threshold = Math.min(active.height * 0.24, 220);
    settleSwipe(Math.abs(active.offset) >= threshold);
  }

  useEffect(() => {
    const handleKeyDown = (event) => {
      if (event.key === "Escape") { setCommentsOpen(false); return; }
      if (isInteractiveTarget(event.target) || gridMode || event.ctrlKey || event.metaKey || event.altKey) return;
      if (["ArrowDown", "ArrowUp"].includes(event.key)) event.preventDefault();
      if (["ArrowDown", "j", "J"].includes(event.key)) {
        moveTo(Math.min(items.length - 1, index + 1));
      }
      if (["ArrowUp", "k", "K"].includes(event.key)) {
        moveTo(Math.max(0, index - 1));
      }
      if (event.key === "l" || event.key === "L") {
        setLike();
      }
      if (event.key === "f" || event.key === "F") {
        setFavorite();
      }
      if (event.key === "r" || event.key === "R") {
        setFollow();
      }
      if (event.key === "c" || event.key === "C") {
        setCommentsOpen(true);
      }
      if (event.key === "Escape") {
        setCommentsOpen(false);
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [index, items.length, setFavorite, setFollow, setLike, swipe, gridMode]);

  function goNext() {
    moveTo(Math.min(items.length - 1, index + 1));
  }

  function goPrev() {
    moveTo(Math.max(0, index - 1));
  }

  function handleWheel(event) {
    if (event.target?.closest?.('.dr-player-info')) return;
    if (Math.abs(event.deltaY) < 32 || wheelLocked.current || swipe || items.length < 2) return;
    wheelLocked.current = true;
    if (event.deltaY > 0) {
      goNext();
    } else {
      goPrev();
    }
    window.setTimeout(() => {
      wheelLocked.current = false;
    }, 420);
  }

  return (
    <main className={`stream-page ${gridMode ? "is-searching" : "is-watching"}`} aria-label={currentFeedScene.label}>
      {feedState === "ready" && feedError && <p className="form-message" role="status">{feedError}</p>}
      {gridMode ? <section className="browse-section"><p className="browse-summary">在已加载的 {items.length} 部作品中找到 {filterVideos(items, query).length} 部{query && <> · “{query}”</>}<button onClick={() => { onClearSearch(); }}>清除筛选</button></p><div className="browse-grid">{filterVideos(items, query).map(({item, index: itemIndex}) => <button className="browse-card" key={item.entry_id || item.video_id} onClick={() => { setIndex(itemIndex); onClearSearch(); }}><div className="browse-thumb"><img src={item.cover_url || image.stage} alt=""/><span className="browse-play"><Icon name="play_arrow" size={25}/></span><span className="browse-model">{item.model_name || "AIGC"}</span></div><h3>{item.title}</h3><div className="browse-meta"><span>@{item.author}</span><span><Icon name="favorite" size={13}/>{formatMetric(item.like_count)}</span></div></button>)}</div>{feedState === "loading" ? <PageMessage icon="hourglass_top" title="正在加载作品"/> : feedState === "error" ? <PageMessage icon="sync_problem" title={feedError} action="重试" onAction={loadFeed}/> : feedState === "auth" ? <PageMessage icon="lock" title="登录后浏览这个视频流" action="登录" onAction={() => onNavigate("/auth")}/> : filterVideos(items, query).length === 0 && <PageMessage icon="search" title="暂时没有匹配的作品，试试其他关键词"/>}{(hasMore || feedScene === "recommend") && feedState === "ready" && <button className="ghost-button browse-more" disabled={loadingMore} onClick={loadMoreFeed}>{loadingMore ? "正在加载…" : "加载更多作品"}</button>}</section> : <>
      <div className={`feed-layout ${commentsOpen ? "has-comments" : ""}`}>
        {feedScene === "hot" && contentSource === "timeline" && feedState === "ready" && items.length > 0 && (
          <span className="feed-source-label" role="status" title="当前暂无实时热门，按近期作品的互动情况展示精选">近期精选</span>
        )}
        <nav className="stream-controls" aria-label="播放列表控制">
          <button type="button" aria-label="上一部视频" title="上一部视频" disabled={index === 0 || Boolean(swipe)} onClick={goPrev}><Icon name="arrow_upward" size={18}/></button>
          <button type="button" aria-label="下一部视频" title="下一部视频" disabled={index >= items.length - 1 || Boolean(swipe)} onClick={goNext}><Icon name="arrow_downward" size={18}/></button>
          <button type="button" aria-label="刷新视频流" title="刷新视频流" disabled={feedState === "loading"} onClick={loadFeed}><Icon name="refresh" size={18}/></button>
        </nav>
      <section
        className="feed-main"
        ref={feedMainRef}
        onWheel={handleWheel}
        onPointerDown={handlePointerDown}
        onPointerMove={handlePointerMove}
        onPointerUp={handlePointerEnd}
        onPointerCancel={handlePointerEnd}
      >
        {feedState === "loading" && <FeedMessage icon="hourglass_top" title={`正在加载${currentFeedScene.label}`} />}
        {feedState === "auth" && (
          <FeedMessage icon="lock" title={`登录后查看${currentFeedScene.label}`} action="登录" onAction={() => onNavigate("/auth")} />
        )}
        {feedState === "error" && (
          <FeedMessage icon="sync_problem" title={feedError} action="重新加载" onAction={loadFeed} />
        )}
        {feedState === "ready" && items.length === 0 && (
          <FeedMessage icon="video_library" title={`${currentFeedScene.label}暂无视频`} action="刷新" onAction={loadFeed} />
        )}
        {visibleCurrent && (
          <div className={`feed-stage-wrap ${swipe ? `swiping ${swipe.direction} ${swipe.settling ? "settling" : "dragging"}` : ""}`}>
            <div className="feed-stage-track" style={trackStyle}>
              {swipe?.direction === "prev" && visibleNext && (
                <div className="feed-stage-layer">
                  <VideoStage
                    key={visibleNext.entry_id || visibleNext.video_id}
                    item={visibleNext}
                    active={false}
                    liked={Boolean(liked[visibleNext.video_id])}
                    favorited={Boolean(favorited[visibleNext.video_id])}
                    following={Boolean(following[visibleNext.author_id])}
                    followBusy={followBusyID === visibleNext.author_id}
                    ownVideo={visibleNext.author_id === session.user?.id}
                    onLike={setLike}
                    onComment={() => setCommentsOpen(true)}
                    onFavorite={setFavorite}
                    onFollow={setFollow}
                    onPlaybackQoS={reportPlaybackQoS}
                    onOpenAuthor={(author) => openPublicProfile(author, onNavigate)}
                    followError={followError}
                  />
                </div>
              )}
              <div className="feed-stage-layer">
                <VideoStage
                  key={visibleCurrent.entry_id || visibleCurrent.video_id}
                  item={visibleCurrent}
                  active={!swipe}
                  liked={Boolean(liked[visibleCurrent.video_id])}
                  favorited={Boolean(favorited[visibleCurrent.video_id])}
                  following={Boolean(following[visibleCurrent.author_id])}
                  followBusy={followBusyID === visibleCurrent.author_id}
                  ownVideo={visibleCurrent.author_id === session.user?.id}
                  onLike={setLike}
                  onComment={() => setCommentsOpen(true)}
                  onFavorite={setFavorite}
                  onFollow={setFollow}
                  onPlaybackQoS={reportPlaybackQoS}
                  onOpenAuthor={(author) => openPublicProfile(author, onNavigate)}
                  followError={followError}
                />
              </div>
              {swipe?.direction === "next" && visibleNext && (
                <div className="feed-stage-layer">
                  <VideoStage
                    key={visibleNext.entry_id || visibleNext.video_id}
                    item={visibleNext}
                    active={false}
                    liked={Boolean(liked[visibleNext.video_id])}
                    favorited={Boolean(favorited[visibleNext.video_id])}
                    following={Boolean(following[visibleNext.author_id])}
                    followBusy={followBusyID === visibleNext.author_id}
                    ownVideo={visibleNext.author_id === session.user?.id}
                    onLike={setLike}
                    onComment={() => setCommentsOpen(true)}
                    onFavorite={setFavorite}
                    onFollow={setFollow}
                    onPlaybackQoS={reportPlaybackQoS}
                    onOpenAuthor={(author) => openPublicProfile(author, onNavigate)}
                    followError={followError}
                  />
                </div>
              )}
            </div>
          </div>
        )}
        {feedState === "ready" && loadingMore && <div className="feed-loading-pill">加载中</div>}
        {feedScene !== "recommend" && feedState === "ready" && items.length > 0 && !hasMore && index === items.length - 1 && (
          <div className="feed-loading-pill">已到末尾</div>
        )}
      </section>
      <CommentPanel
        open={commentsOpen}
        value={commentText}
        onChange={setCommentText}
        onClose={() => setCommentsOpen(false)}
        onSubmit={submitComment}
        user={session.user || emptyProfile}
        count={current?.comment_count || 0}
        comments={comments}
        state={commentsState}
        error={commentsError}
        onRetry={loadComments}
        authenticated={Boolean(session.token && session.user)}
        onOpenUser={(profile) => openPublicProfile(profile, onNavigate)}
      />
    </div>
      </>}
    </main>
  );
}

function FeedMessage({ icon, title, action, onAction }) {
  return (
    <div className="feed-message">
      <Icon name={icon}/>
      <strong>{title}</strong>
      {action && <button onClick={onAction}>{action}</button>}
    </div>
  );
}

function CommentPanel({
  open,
  value,
  onChange,
  onSubmit,
  onClose,
  user,
  count,
  comments,
  state,
  error,
  onRetry,
  authenticated,
  onOpenUser
}) {
  return (
    <aside className={`comment-panel ${open ? "open" : ""}`}>
      <header className="comment-header">
        <h2>
          评论 <span>{formatMetric(count)}</span>
        </h2>
        <div>

          <button className="icon-button small" type="button" aria-label="关闭评论" onClick={onClose}>
            <Icon name="close"/>
          </button>
        </div>
      </header>
      <div className="comment-list">
        {state === "loading" && <CommentMessage icon="hourglass_top" title="正在加载评论" />}
        {state === "error" && <CommentMessage icon="sync_problem" title={error || "评论加载失败"} action="重试" onAction={onRetry} />}
        {state === "ready" && comments.length === 0 && <CommentMessage icon="chat_bubble" title="暂无评论" />}
        {comments.map((comment) => (
          <article className="comment-item" key={comment.id}>
            <button className="comment-user-button" type="button" onClick={() => onOpenUser(profileFromComment(comment))}>
              <img src={comment.user_avatar_url || image.currentUser} alt="" />
            </button>
            <div>
              <div className="comment-meta">
                <button type="button" onClick={() => onOpenUser(profileFromComment(comment))}>
                  {comment.user_nickname || `用户_${comment.user_id}`}
                </button>
                <span>{formatRelativeTime(comment.created_at)}</span>
              </div>
              <p>{comment.content}</p>
            </div>
          </article>
        ))}
      </div>
      <form
        className="comment-form"
        onSubmit={(event) => {
          event.preventDefault();
          onSubmit();
        }}
      >
        <img src={user.avatar_url || image.currentUser} alt="" />
        <input
          value={value}
          onChange={(event) => onChange(event.target.value)}
          placeholder={authenticated ? "添加评论..." : "登录后评论"}
          disabled={!authenticated}
        />
        <button aria-label="发送评论" disabled={!authenticated || !value.trim()}>
          <Icon name="send"/>
        </button>
      </form>
    </aside>
  );
}

function CommentMessage({ icon, title, action, onAction }) {
  return (
    <div className="comment-empty">
      <Icon name={icon}/>
      <strong>{title}</strong>
      {action && <button onClick={onAction}>{action}</button>}
    </div>
  );
}

function MessagesPage({ session, onNavigate, onUnreadChange }) {
  const [items, setItems] = useState([]);
  const [nextCursor, setNextCursor] = useState("");
  const [hasMore, setHasMore] = useState(false);
  const [state, setState] = useState("loading");
  const [error, setError] = useState("");
  const [busyID, setBusyID] = useState(0);
  const [markingAll, setMarkingAll] = useState(false);

  const loadMessages = useCallback(
    (cursor = "", append = false) => {
      if (!session.token) {
        onNavigate("/auth");
        return Promise.resolve();
      }
      setState(append ? "loadingMore" : "loading");
      setError("");
      return fetchMessages(session.token, cursor)
        .then((data) => {
          const nextItems = data.items || [];
          setItems((current) => (append ? appendMessages(current, nextItems) : nextItems));
          setNextCursor(data.next_cursor || "");
          setHasMore(Boolean(data.has_more && data.next_cursor));
          setState("ready");
        })
        .catch((loadError) => {
          if (loadError.status === 401) {
            session.clearAuth();
            onNavigate("/auth");
            return;
          }
          setError(loadError.message || "消息加载失败");
          setState("error");
        });
    },
    [onNavigate, session]
  );

  useEffect(() => {
    loadMessages("", false);
  }, [loadMessages]);

  async function markMessageRead(message) {
    if (!message || message.is_read || busyID || markingAll) return;
    setBusyID(message.id);
    try {
      await markMessagesRead(session.token, [message.id]);
      setItems((current) =>
        current.map((item) => (item.id === message.id ? { ...item, is_read: true, read_at: new Date().toISOString() } : item))
      );
      await onUnreadChange();
    } catch (markError) {
      if (markError.status === 401) {
        session.clearAuth();
        onNavigate("/auth");
        return;
      }
      setError(markError.message || "已读操作失败");
    } finally {
      setBusyID(0);
    }
  }

  async function markAllRead() {
    if (markingAll || items.every((item) => item.is_read)) return;
    setMarkingAll(true);
    setError("");
    try {
      await markMessagesRead(session.token, []);
      setItems((current) => current.map((item) => ({ ...item, is_read: true, read_at: item.read_at || new Date().toISOString() })));
      await onUnreadChange();
    } catch (markError) {
      if (markError.status === 401) {
        session.clearAuth();
        onNavigate("/auth");
        return;
      }
      setError(markError.message || "全部已读失败");
    } finally {
      setMarkingAll(false);
    }
  }

  const unreadCount = items.filter((item) => !item.is_read).length;
  const loadingInitial = state === "loading" && items.length === 0;
  const loadingMore = state === "loadingMore";

  return (
    <main className="messages-page">
      <section className="messages-header">
        <div>
          <p className="eyebrow">Messages</p>
          <h1>消息中心</h1>
        </div>
        <div className="messages-actions">
          <span className="messages-count">{unreadCount > 0 ? `${unreadCount} 未读` : "已读完"}</span>
          <button className="ghost-button compact" onClick={() => loadMessages("", false)} disabled={loadingInitial || loadingMore}>
            <Icon name="refresh"/>
            刷新
          </button>
          <button className="primary-button compact" onClick={markAllRead} disabled={markingAll || unreadCount === 0}>
            <Icon name="done_all"/>
            {markingAll ? "处理中" : "全部已读"}
          </button>
        </div>
      </section>

      <section className="messages-list-wrap">
        {loadingInitial && <PageMessage icon="hourglass_top" title="正在加载消息" />}
        {state === "error" && items.length === 0 && (
          <PageMessage icon="sync_problem" title={error || "消息加载失败"} action="重试" onAction={() => loadMessages("", false)} />
        )}
        {state === "ready" && items.length === 0 && <PageMessage icon="notifications" title="暂无消息" />}
        {error && items.length > 0 && <p className="form-message">{error}</p>}
        <div className="messages-list">
          {items.map((message) => {
            const actor = messageActor(message);
            const body = messageBody(message);
            return (
              <button
                className={`message-item ${message.is_read ? "read" : "unread"}`}
                key={message.id}
                type="button"
                onClick={() => markMessageRead(message)}
                disabled={busyID === message.id}
              >
                <span className={`message-icon ${message.is_read ? "" : "active"}`}>
                  <Icon name={messageIcon(message.type)}/>
                </span>
                <span className="message-copy">
                  <span className="message-title-row">
                    <strong>{message.title}</strong>
                    <small>{formatRelativeTime(message.created_at)}</small>
                  </span>
                  {actor && (
                    <span className="message-actor-row">
                      <img src={actor.avatar_url} alt="" />
                      <strong>{actor.nickname}</strong>
                    </span>
                  )}
                  <span className="message-content-text">{body}</span>
                </span>
                <span className="message-state">{message.is_read ? "已读" : busyID === message.id ? "处理中" : "未读"}</span>
              </button>
            );
          })}
        </div>
        {hasMore && (
          <button className="ghost-button messages-more" onClick={() => loadMessages(nextCursor, true)} disabled={loadingMore}>
            <Icon name="expand_more"/>
            {loadingMore ? "加载中" : "加载更多"}
          </button>
        )}
      </section>
    </main>
  );
}

function PageMessage({ icon, title, action, onAction }) {
  return (
    <div className="page-message">
      <Icon name={icon}/>
      <strong>{title}</strong>
      {action && <button onClick={onAction}>{action}</button>}
    </div>
  );
}

function ProfilePage({ session, onNavigate, editing, setEditing }) {
  const baseUser = session.user || emptyProfile;
  const [form, setForm] = useState({
    nickname: baseUser.nickname || "",
    avatar_url: baseUser.avatar_url || "",
    bio: baseUser.bio || ""
  });
  const [avatarFile, setAvatarFile] = useState(null);
  const [avatarPreview, setAvatarPreview] = useState("");
  const [selectedWork, setSelectedWork] = useState(null);
  const [status, setStatus] = useState("");
  const [videos, setVideos] = useState([]);
  const [videosState, setVideosState] = useState("loading");
  const [relationTab, setRelationTab] = useState("following");
  const [relationModalOpen, setRelationModalOpen] = useState(false);
  const [relationItems, setRelationItems] = useState([]);
  const [relationCursor, setRelationCursor] = useState("");
  const [relationHasMore, setRelationHasMore] = useState(false);
  const [relationState, setRelationState] = useState("idle");
  const [relationError, setRelationError] = useState("");
  const [relationFollowing, setRelationFollowing] = useState({});
  const [relationBusyID, setRelationBusyID] = useState(0);
  const followingCount = baseUser.following_count ?? baseUser.followingCount ?? 0;
  const followerCount = baseUser.follower_count ?? baseUser.followerCount ?? 0;

  useEffect(() => {
    setForm({
      nickname: baseUser.nickname || "",
      avatar_url: baseUser.avatar_url || "",
      bio: baseUser.bio || ""
    });
    setAvatarFile(null);
    setAvatarPreview("");
  }, [baseUser.avatar_url, baseUser.bio, baseUser.nickname]);

  useEffect(() => {
    if (!avatarFile) {
      setAvatarPreview("");
      return;
    }
    const objectURL = URL.createObjectURL(avatarFile);
    setAvatarPreview(objectURL);
    return () => URL.revokeObjectURL(objectURL);
  }, [avatarFile]);

  useEffect(() => {
    if (!session.token) {
      setVideosState("ready");
      return;
    }
    setVideosState("loading");
    apiRequest("/api/users/me/videos?limit=12", { token: session.token })
      .then((data) => {
        setVideos(data.items || []);
        setVideosState("ready");
      })
      .catch((error) => {
        if (error.status === 401) {
          session.clearAuth();
          onNavigate("/auth");
          return;
        }
        setVideos([]);
        setVideosState(error.message || "作品加载失败");
      });
  }, [onNavigate, session]);

  useEffect(() => {
    if (!session.token) {
      setRelationItems([]);
      setRelationCursor("");
      setRelationHasMore(false);
      setRelationFollowing({});
      setRelationState("ready");
      return undefined;
    }

    let live = true;
    loadFollowingMap(session.token)
      .then((map) => {
        if (live) {
          setRelationFollowing(map);
        }
      })
      .catch((error) => {
        if (error.status === 401) {
          session.clearAuth();
          onNavigate("/auth");
        }
      });
    return () => {
      live = false;
    };
  }, [onNavigate, session]);

  const loadRelationPage = useCallback(
    async ({ reset = false, cursor = "" } = {}) => {
      if (!session.token) return;
      const requestCursor = reset ? "" : cursor;
      setRelationState(reset ? "loading" : "loadingMore");
      setRelationError("");
      try {
        const path = relationListPath(relationTab, requestCursor);
        const data = await apiRequest(path, { token: session.token });
        const items = data.items || [];
        setRelationItems((state) => (reset ? items : [...state, ...items]));
        setRelationCursor(data.next_cursor || "");
        setRelationHasMore(Boolean(data.has_more));
        if (relationTab === "following") {
          setRelationFollowing((state) => {
            const next = { ...state };
            for (const item of items) {
              next[item.user_id] = true;
            }
            return next;
          });
        }
        setRelationState("ready");
      } catch (error) {
        if (error.status === 401) {
          session.clearAuth();
          onNavigate("/auth");
          return;
        }
        setRelationError(error.message || "关系列表加载失败");
        setRelationState("error");
      }
    },
    [onNavigate, relationTab, session]
  );

  useEffect(() => {
    setRelationItems([]);
    setRelationCursor("");
    setRelationHasMore(false);
    if (!session.token || !relationModalOpen) return;
    loadRelationPage({ reset: true });
  }, [loadRelationPage, relationModalOpen, relationTab, session.token]);

  function openRelationModal(tab) {
    setRelationTab(tab);
    setRelationModalOpen(true);
  }

  async function toggleRelationFollow(targetUserID) {
    if (!session.token) {
      onNavigate("/auth");
      return;
    }
    if (!targetUserID || targetUserID === baseUser.id) return;

    const currentFollowing = Boolean(relationFollowing[targetUserID]);
    setRelationBusyID(targetUserID);
    setRelationError("");
    try {
      const data = await apiRequest(`/api/users/me/following/${targetUserID}`, {
        method: currentFollowing ? "DELETE" : "PUT",
        token: session.token,
        headers: {
          "Idempotency-Key": `web-profile-follow-${targetUserID}-${Date.now()}`
        }
      });
      setRelationFollowing((state) => ({ ...state, [targetUserID]: Boolean(data.following) }));
      if (relationTab === "following" && !data.following) {
        setRelationItems((state) => state.filter((item) => item.user_id !== targetUserID));
      }
      updateSessionRelationCount(session, data.following_count);
      setRelationState("ready");
    } catch (error) {
      if (error.status === 401) {
        session.clearAuth();
        onNavigate("/auth");
        return;
      }
      setRelationError(error.message || "关注操作失败");
      setRelationState("error");
    } finally {
      setRelationBusyID(0);
    }
  }

  async function handleSave(event) {
    event.preventDefault();
    setStatus("保存中");
    try {
      let avatarURL = form.avatar_url;
      if (avatarFile && session.token) {
        const uploaded = await uploadFile(avatarFile, "avatar", session.token);
        avatarURL = uploaded.url;
      }
      let profile = { ...baseUser, ...form };
      if (session.token) {
        profile = await apiRequest("/api/users/me", {
          method: "PATCH",
          token: session.token,
          body: {
            nickname: form.nickname,
            avatar_url: avatarURL,
            bio: form.bio
          }
        });
      }
      session.setAuth(session.token, profile);
      setAvatarFile(null);
      setStatus("已保存");
      setEditing(false);
    } catch (error) {
      setStatus(error.message || "保存失败");
    }
  }

  return (
    <main className="profile-page">
      <section className="profile-hero">
        <div className="profile-summary">
          <img className="profile-avatar" src={avatarPreview || form.avatar_url || image.currentUser} alt="" />
          <div>
            <p className="eyebrow">创作者资料</p>
            <h1>{form.nickname || baseUser.account}</h1>
            <p>{form.bio || "作品、关注和互动资料会显示在这里。"}</p>
          </div>
          <div className="profile-stats" aria-label="资料统计">
            <button
              className={relationModalOpen && relationTab === "following" ? "active" : ""}
              type="button"
              onClick={() => openRelationModal("following")}
            >
              <strong>{formatMetric(followingCount)}</strong>
              关注
            </button>
            <button
              className={relationModalOpen && relationTab === "followers" ? "active" : ""}
              type="button"
              onClick={() => openRelationModal("followers")}
            >
              <strong>{formatMetric(followerCount)}</strong>
              粉丝
            </button>
            <button type="button">
              <strong>{formatMetric(videos.length)}</strong>
              作品
            </button>
          </div>
        </div>
      </section>

      <section className="profile-grid">
        <section className="profile-card works-card">
          <header>
            <h2>我的作品</h2>
            <button className="ghost-button compact" onClick={() => onNavigate("/timeline")}>
              <Icon name="home"/>
              最新视频
            </button>
          </header>
          <VideoGrid videos={videos} state={videosState} onSelect={setSelectedWork} />
        </section>
      </section>
      {relationModalOpen && (
        <RelationModal
          tab={relationTab}
          items={relationItems}
          state={relationState}
          error={relationError}
          hasMore={relationHasMore}
          following={relationFollowing}
          busyID={relationBusyID}
          currentUserID={baseUser.id}
          onTabChange={setRelationTab}
          onClose={() => setRelationModalOpen(false)}
          onRetry={() => loadRelationPage({ reset: true })}
          onLoadMore={() => loadRelationPage({ reset: false, cursor: relationCursor })}
          onToggleFollow={toggleRelationFollow}
        />
      )}
      {selectedWork && <WorkViewer video={selectedWork} onClose={() => setSelectedWork(null)} />}
      {editing && (
        <div className="modal-backdrop" role="presentation">
          <form className="profile-modal profile-form" role="dialog" aria-modal="true" aria-labelledby="profile-edit-title" onSubmit={handleSave} onKeyDown={(event) => {
            if (event.key === "Escape") { event.preventDefault(); setEditing(false); document.getElementById("account-trigger")?.focus(); }
            if (event.key === "Tab") {
              const fields = [...event.currentTarget.querySelectorAll('button:not(:disabled), input:not(:disabled), textarea:not(:disabled)')];
              const first = fields[0], last = fields[fields.length - 1];
              if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus(); }
              else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus(); }
            }
          }}>
            <header>
              <h2 id="profile-edit-title">编辑个人资料</h2>
              <button className="icon-button small" type="button" onClick={() => setEditing(false)} aria-label="关闭">
                <Icon name="close"/>
              </button>
            </header>
            <label>
              <span>昵称</span>
              <input autoFocus value={form.nickname} onChange={(event) => setForm({ ...form, nickname: event.target.value })} />
            </label>
            <label>
              <span>头像</span>
              <span className="file-picker avatar-picker">
                <span className="avatar-upload-preview">
                  {avatarPreview || form.avatar_url ? (
                    <img src={avatarPreview || form.avatar_url} alt="" />
                  ) : (
                    <Icon name="person"/>
                  )}
                </span>
                <span className="file-picker-copy">
                  <strong>{avatarFile ? avatarFile.name : "选择头像文件"}</strong>
                  <small>本地图片上传</small>
                </span>
                <input type="file" accept="image/*" onChange={(event) => setAvatarFile(event.target.files?.[0] || null)} />
              </span>
            </label>
            <label>
              <span>简介</span>
              <textarea value={form.bio} onChange={(event) => setForm({ ...form, bio: event.target.value })} rows={4} />
            </label>
            {status && <p className={`form-message ${status === "已保存" ? "success" : ""}`}>{status}</p>}
            <button className="primary-button">
              <Icon name="save"/>
              保存
            </button>
          </form>
        </div>
      )}
    </main>
  );
}

function PublicProfilePage({ userID, onNavigate }) {
  const [profile, setProfile] = useState(() => readPublicProfile(userID));
  const [profileState, setProfileState] = useState("idle");
  const [videos, setVideos] = useState([]);
  const [videosState, setVideosState] = useState("loading");
  const [selectedWork, setSelectedWork] = useState(null);

  useEffect(() => {
    setProfile(readPublicProfile(userID));
  }, [userID]);

  useEffect(() => {
    let live = true;
    setProfileState("loading");
    apiRequest(`/api/users/${userID}`)
      .then((data) => {
        if (!live) return;
        const nextProfile = normalizePublicProfile(data);
        setProfile(nextProfile);
        if (nextProfile) {
          savePublicProfile(nextProfile);
        }
        setProfileState("ready");
      })
      .catch((error) => {
        if (!live) return;
        setProfileState(error.message || "资料加载失败");
      });
    return () => {
      live = false;
    };
  }, [userID]);

  useEffect(() => {
    let live = true;
    setVideosState("loading");
    apiRequest(`/api/users/${userID}/videos?limit=24`)
      .then((data) => {
        if (!live) return;
        setVideos(data.items || []);
        setVideosState("ready");
      })
      .catch((error) => {
        if (!live) return;
        setVideos([]);
        setVideosState(error.message || "作品加载失败");
      });
    return () => {
      live = false;
    };
  }, [userID]);

  const displayProfile = profile || {
    id: userID,
    nickname: `用户_${userID}`,
    avatar_url: image.currentUser,
    bio: "这个用户的资料会显示在这里。"
  };

  return (
    <main className="profile-page">
      <section className="profile-hero">
        <div className="profile-summary public-profile-summary">
          <img className="profile-avatar" src={displayProfile.avatar_url || image.currentUser} alt="" />
          <div>
            <p className="eyebrow">用户主页</p>
            <h1>{displayProfile.nickname || `用户_${userID}`}</h1>
            <p>{displayProfile.bio || "这个用户还没有填写简介。"}</p>
            {profileState !== "idle" && profileState !== "loading" && profileState !== "ready" && (
              <p className="form-message">{profileState}</p>
            )}
          </div>
          <div className="profile-stats public-profile-stats" aria-label="资料统计">
            <button type="button">
              <strong>{formatOptionalMetric(displayProfile.following_count)}</strong>
              关注
            </button>
            <button type="button">
              <strong>{formatOptionalMetric(displayProfile.follower_count)}</strong>
              粉丝
            </button>
            <button type="button">
              <strong>{formatOptionalMetric(displayProfile.work_count)}</strong>
              作品
            </button>
          </div>
          <button className="ghost-button compact public-back-button" type="button" onClick={() => onNavigate("/timeline")}>
            <Icon name="home"/>
            最新视频
          </button>
        </div>
      </section>

      <section className="profile-grid">
        <section className="profile-card works-card">
          <header>
            <h2>他的作品</h2>
          </header>
          <VideoGrid videos={videos} state={videosState} onSelect={setSelectedWork} />
        </section>
      </section>
      {selectedWork && <WorkViewer video={selectedWork} onClose={() => setSelectedWork(null)} />}
    </main>
  );
}

function VideoGrid({ videos, state, onSelect }) {
  return (
    <div className="work-list">
      {state === "loading" && <p className="card-empty">正在加载作品</p>}
      {state !== "loading" && typeof state === "string" && state !== "ready" && <p className="card-empty">{state}</p>}
      {state === "ready" && videos.length === 0 && <p className="card-empty">暂无作品</p>}
      {videos.map((video) => (
        <button className="work-item" key={video.id || video.video_id} onClick={() => onSelect(video)}>
          <div className="work-thumb">
            <img src={video.cover_url || image.stage} alt="" />
            <Icon name="play_arrow"/>
          </div>
          <div className="work-meta">
            <h3>{video.title}</h3>
            <p>{formatMetric(video.like_count || 0)} 点赞 · {formatMetric(video.comment_count || 0)} 评论</p>
            <span className="status-badge">{video.status === 0 ? "审核中" : "已发布"}</span>
          </div>
        </button>
      ))}
    </div>
  );
}

function RelationList({
  tab,
  items,
  state,
  error,
  hasMore,
  following,
  busyID,
  currentUserID,
  onRetry,
  onLoadMore,
  onToggleFollow
}) {
  const loading = state === "loading";
  const loadingMore = state === "loadingMore";

  if (loading) {
    return <p className="card-empty">正在加载关系</p>;
  }

  if (state === "error" && items.length === 0) {
    return (
      <div className="card-empty">
        <span>{error || "关系列表加载失败"}</span>
        <button className="ghost-button compact" type="button" onClick={onRetry}>
          重试
        </button>
      </div>
    );
  }

  return (
    <div className="relation-list-wrap">
      {items.length === 0 && <p className="card-empty">{tab === "following" ? "暂无关注" : "暂无粉丝"}</p>}
      <div className="relation-list">
        {items.map((item) => {
          const isSelf = item.user_id === currentUserID;
          const isFollowing = Boolean(following[item.user_id]);
          return (
            <article className="relation-item" key={`${tab}-${item.user_id}`}>
              <img src={item.avatar_url || image.currentUser} alt="" />
              <div>
                <strong>{item.nickname || `用户_${item.user_id}`}</strong>
                <p>{item.bio || "这个用户还没有填写简介。"}</p>
                <span>{formatRelativeTime(item.followed_at)}</span>
              </div>
              <button
                className={`relation-follow-button ${isFollowing ? "active" : ""}`}
                type="button"
                onClick={() => onToggleFollow(item.user_id)}
                disabled={busyID === item.user_id || isSelf}
              >
                {isSelf ? "本人" : busyID === item.user_id ? "处理中" : isFollowing ? "已关注" : "关注"}
              </button>
            </article>
          );
        })}
      </div>
      {state === "error" && items.length > 0 && <p className="form-message">{error || "加载失败"}</p>}
      {hasMore && (
        <button className="ghost-button compact relation-more-button" type="button" onClick={onLoadMore} disabled={loadingMore}>
          {loadingMore ? "加载中" : "加载更多"}
        </button>
      )}
    </div>
  );
}

function RelationModal({
  tab,
  items,
  state,
  error,
  hasMore,
  following,
  busyID,
  currentUserID,
  onTabChange,
  onClose,
  onRetry,
  onLoadMore,
  onToggleFollow
}) {
  return (
    <div className="modal-backdrop relation-modal-backdrop" role="presentation" onClick={onClose}>
      <section className="relation-modal" onClick={(event) => event.stopPropagation()}>
        <header>
          <div>
            <p className="eyebrow">关系</p>
            <h2>{tab === "following" ? "关注的人" : "粉丝"}</h2>
          </div>
          <div className="relation-modal-actions">
            <div className="relation-tabs">
              <button className={tab === "following" ? "active" : ""} type="button" onClick={() => onTabChange("following")}>
                关注
              </button>
              <button className={tab === "followers" ? "active" : ""} type="button" onClick={() => onTabChange("followers")}>
                粉丝
              </button>
            </div>
            <button className="icon-button small" type="button" onClick={onClose} aria-label="关闭关系弹窗">
              <Icon name="close"/>
            </button>
          </div>
        </header>
        <RelationList
          tab={tab}
          items={items}
          state={state}
          error={error}
          hasMore={hasMore}
          following={following}
          busyID={busyID}
          currentUserID={currentUserID}
          onRetry={onRetry}
          onLoadMore={onLoadMore}
          onToggleFollow={onToggleFollow}
        />
      </section>
    </div>
  );
}

function WorkViewer({ video, onClose }) {
  const media = video.media_url || video.cover_url || image.stage;
  const cover = video.cover_url || image.stage;

  return (
    <div className="modal-backdrop work-viewer-backdrop" role="presentation" onClick={onClose}>
      <section className="work-viewer" onClick={(event) => event.stopPropagation()}>
        <header>
          <div>
            <h2>{video.title || "作品"}</h2>
            <p>{formatMetric(video.like_count || 0)} 点赞 · {formatMetric(video.comment_count || 0)} 评论</p>
          </div>
          <button className="icon-button small" type="button" onClick={onClose} aria-label="关闭">
            <Icon name="close"/>
          </button>
        </header>
        <div className="work-viewer-stage">
          {isVideoSource(media) ? (
            <video src={media} poster={cover} controls autoPlay playsInline />
          ) : (
            <img src={cover} alt="" />
          )}
        </div>
        <AIGCMetadata video={video} />
      </section>
    </div>
  );
}

function normalizeRoute(pathname) {
  if (pathname === "/login") return "/auth";
  if (pathname === "/me") return "/profile";
  if (pathname === "/upload") return "/create";
  if (/^\/users\/\d+$/.test(pathname)) return pathname;
  if (["/", "/auth", "/recommend", "/timeline", "/following", "/hotfeed", "/models", "/profile", "/create", "/upload", "/messages"].includes(pathname)) {
    return pathname;
  }
  return "/timeline";
}

function feedSceneFromRoute(route) {
  const scene = FEED_SCENES.find((item) => item.route === route);
  return scene?.key || FEED_SCENES[0].key;
}

function navigate(path, setRoute) {
  const nextPath = normalizeRoute(path);
  window.history.pushState({}, "", nextPath);
  setRoute(nextPath);
}

function logout(session, setRoute) {
  if (session.token) {
    apiRequest("/api/sessions/current", {
      method: "DELETE",
      token: session.token
    }).catch(() => {});
  }
  session.clearAuth();
  navigate("/timeline", setRoute);
}

function requiresAuthFeed(scene) {
  return scene === "following" || scene === "recommend";
}

function createFeedRequestID(scene) {
  const random = Math.random().toString(36).slice(2, 8);
  return `web-${scene}-${Date.now()}-${random}`;
}

async function fetchFeedPage(scene, token, cursor = "", requestID = "", signal) {
  const limit = 10;
  if (scene === "recommend") {
    return apiRequest("/api/feed-queries", {
      method: "POST",
      token,
      signal,
      body: {
        scene,
        cursor,
        limit,
        context: {
          request_id: requestID
        }
      }
    });
  }

  const params = new URLSearchParams({ scene, limit: String(limit) });
  if (cursor) {
    params.set("cursor", cursor);
  }
  return apiRequest(`/api/feed-items?${params.toString()}`, { token, signal });
}

async function fetchMessages(token, cursor = "") {
  const params = new URLSearchParams({ limit: "20" });
  if (cursor) {
    params.set("cursor", cursor);
  }
  return apiRequest(`/api/messages?${params.toString()}`, { token });
}

async function fetchPlaybackConfig(token) {
  const params = new URLSearchParams({
    platform: "Web",
    network_type: detectNetworkType()
  });
  return apiRequest(`/api/playback-config?${params.toString()}`, { token });
}

async function fetchPreloadVideos(token, currentVideoID, limit) {
  const params = new URLSearchParams({
    current_video_id: String(currentVideoID || 0),
    limit: String(limit || DEFAULT_PLAYBACK_CONFIG.preload_count)
  });
  return apiRequest(`/api/preload-videos?${params.toString()}`, { token });
}

async function markMessagesRead(token, messageIDs = []) {
  return apiRequest("/api/messages", {
    method: "PATCH",
    token,
    body: {
      message_ids: messageIDs
    }
  });
}

function appendMessages(currentItems, nextItems) {
  if (!nextItems.length) return currentItems;
  const seen = new Set(currentItems.map((item) => item.id));
  const merged = [...currentItems];
  for (const item of nextItems) {
    if (seen.has(item.id)) continue;
    seen.add(item.id);
    merged.push(item);
  }
  return merged;
}

function viewerActionMap(items, field) {
  const map = {};
  for (const item of items) {
    if (item?.video_id && item[field]) {
      map[item.video_id] = true;
    }
  }
  return map;
}

function mergeViewerActions(items, setMap, field) {
  if (!items.length) return;
  setMap((state) => {
    const next = { ...state };
    for (const item of items) {
      if (item?.video_id) {
        next[item.video_id] = Boolean(item[field]);
      }
    }
    return next;
  });
}

function normalizePlaybackConfig(config) {
  const preloadCount = Number(config?.preload_count);
  const bufferMs = Number(config?.buffer_ms);
  return {
    platform: config?.platform || DEFAULT_PLAYBACK_CONFIG.platform,
    network_type: config?.network_type || DEFAULT_PLAYBACK_CONFIG.network_type,
    preload_count: Number.isFinite(preloadCount) ? Math.max(1, Math.min(10, preloadCount)) : DEFAULT_PLAYBACK_CONFIG.preload_count,
    buffer_ms: Number.isFinite(bufferMs) && bufferMs > 0 ? bufferMs : DEFAULT_PLAYBACK_CONFIG.buffer_ms
  };
}

function detectNetworkType() {
  const connection = navigator.connection || navigator.mozConnection || navigator.webkitConnection;
  const raw = String(connection?.effectiveType || connection?.type || "").toLowerCase();
  if (raw.includes("wifi")) return "WiFi";
  if (raw.includes("5g")) return "5G";
  if (raw.includes("4g")) return "4G";
  if (raw.includes("3g")) return "3G";
  return DEFAULT_PLAYBACK_CONFIG.network_type;
}

function prewarmVideoAssets(items, loadedSet) {
  for (const item of items) {
    const coverKey = `cover:${item.cover_url || ""}`;
    if (item.cover_url && !loadedSet.has(coverKey)) {
      const imagePreload = new Image();
      imagePreload.src = item.cover_url;
      loadedSet.add(coverKey);
    }
    prewarmVideoMetadata(item.media_url, loadedSet);
  }
  // 长时间续刷时只保留最近一批预加载标记。
  if (loadedSet.size > FEED_BUFFER_LIMIT * 4) {
    for (const key of [...loadedSet].slice(0, loadedSet.size - FEED_BUFFER_LIMIT * 2)) loadedSet.delete(key);
  }
}

function prewarmVideoMetadata(url, loadedSet) {
  const mediaURL = String(url || "").trim();
  const mediaKey = `metadata:${mediaURL}`;
  if (!mediaURL || loadedSet.has(mediaKey) || typeof document === "undefined") return;
  loadedSet.add(mediaKey);

  const probe = document.createElement("video");
  probe.preload = "metadata";
  probe.src = mediaURL;
  probe.muted = true;
  probe.playsInline = true;
  probe.setAttribute("aria-hidden", "true");
  probe.style.position = "absolute";
  probe.style.width = "1px";
  probe.style.height = "1px";
  probe.style.opacity = "0";
  probe.style.pointerEvents = "none";

  let timer = 0;
  const cleanup = () => {
    window.clearTimeout(timer);
    probe.removeEventListener("loadedmetadata", cleanup);
    probe.removeEventListener("canplay", cleanup);
    probe.removeEventListener("error", cleanup);
    probe.removeAttribute("src");
    probe.load();
    probe.remove();
  };

  probe.addEventListener("loadedmetadata", cleanup);
  probe.addEventListener("canplay", cleanup);
  probe.addEventListener("error", cleanup);
  timer = window.setTimeout(cleanup, 4000);
  document.body.appendChild(probe);
  probe.load();
}

function buildPlaybackQoSPayload(metrics) {
  const watchMs = Math.max(0, Math.round(Number(metrics?.watchMs || 0)));
  const stutterCount = Math.max(0, Math.round(Number(metrics?.stutterCount || 0)));
  const firstFrameNumber = Number(metrics?.firstFrameMs);
  const firstFrameMs = Number.isFinite(firstFrameNumber) ? Math.max(0, Math.round(firstFrameNumber)) : undefined;
  if (watchMs <= 0 && stutterCount <= 0 && firstFrameMs === undefined) return null;
  return {
    ...(firstFrameMs === undefined ? {} : { first_frame_ms: firstFrameMs }),
    stutter_count: stutterCount,
    watch_ms: watchMs
  };
}

function createPlaybackQoSKey(item, metrics) {
  return `web-qos-${item.video_id}-${metrics?.reportID || Date.now()}`;
}

function readStoredUser() {
  try {
    const raw = localStorage.getItem(USER_KEY);
    return raw ? JSON.parse(raw) : null;
  } catch {
    return null;
  }
}

function publicUserIDFromRoute(route) {
  const match = /^\/users\/(\d+)$/.exec(route);
  if (!match) return 0;
  return Number(match[1]);
}

function openPublicProfile(profile, onNavigate) {
  const normalized = normalizePublicProfile(profile);
  if (!normalized?.id) return;
  savePublicProfile(normalized);
  onNavigate(`/users/${normalized.id}`);
}

function normalizePublicProfile(profile) {
  if (!profile) return null;
  const id = Number(profile.id || profile.user_id || profile.author_id || 0);
  if (!id) return null;
  const followingCount = valueOrUndefined(profile.following_count ?? profile.followingCount);
  const followerCount = valueOrUndefined(profile.follower_count ?? profile.followerCount);
  return {
    id,
    nickname: profile.nickname || profile.author || profile.user_nickname || `用户_${id}`,
    avatar_url: profile.avatar_url || profile.user_avatar_url || profile.author_avatar_url || image.currentUser,
    bio: profile.bio || profile.description || "",
    work_count: valueOrUndefined(profile.work_count ?? profile.workCount),
    ...(followingCount === undefined ? {} : { following_count: followingCount }),
    ...(followerCount === undefined ? {} : { follower_count: followerCount })
  };
}

function valueOrUndefined(value) {
  if (value === undefined || value === null || value === "") return undefined;
  const number = Number(value);
  return Number.isFinite(number) ? number : undefined;
}

function profileFromComment(comment) {
  return {
    id: comment.user_id,
    nickname: comment.user_nickname || `用户_${comment.user_id}`,
    avatar_url: comment.user_avatar_url || image.currentUser,
    bio: ""
  };
}

function messageActor(message) {
  const id = Number(message?.actor_id || 0);
  const nickname = (message?.actor_nickname || legacyMessageActorName(message?.content) || (id ? `用户_${id}` : "")).trim();
  if (!id && !nickname) return null;
  return {
    id,
    nickname: nickname || `用户_${id}`,
    avatar_url: message?.actor_avatar_url || image.currentUser
  };
}

function messageBody(message) {
  const content = String(message?.content || "").trim();
  if (message?.actor_id || message?.actor_nickname) return content;
  return content.replace(/^用户\s*\d+\s*/, "").trim() || content;
}

function legacyMessageActorName(content) {
  const match = /^用户\s*(\d+)/.exec(String(content || "").trim());
  return match ? `用户_${match[1]}` : "";
}

function messageIcon(type) {
  switch (String(type || "").toUpperCase()) {
    case "LIKE":
      return "favorite";
    case "COMMENT":
      return "chat_bubble";
    case "FOLLOW":
      return "person_add";
    case "SYSTEM":
      return "campaign";
    default:
      return "notifications";
  }
}

function formatBadgeCount(count) {
  const value = Number(count || 0);
  if (!Number.isFinite(value) || value <= 0) return "";
  return value > 99 ? "99+" : String(value);
}

function readPublicProfiles() {
  try {
    const raw = localStorage.getItem(PUBLIC_PROFILE_KEY);
    return raw ? JSON.parse(raw) : {};
  } catch {
    return {};
  }
}

function readPublicProfile(userID) {
  return readPublicProfiles()[String(userID)] || null;
}

function savePublicProfile(profile) {
  const profiles = readPublicProfiles();
  profiles[String(profile.id)] = profile;
  localStorage.setItem(PUBLIC_PROFILE_KEY, JSON.stringify(profiles));
}

async function apiRequest(path, options = {}) {
  const headers = {
    Accept: "application/json",
    ...(options.headers || {})
  };
  if (options.body) headers["Content-Type"] = "application/json";
  if (options.token) headers.Authorization = `Bearer ${options.token}`;

  const response = await fetch(path, {
    method: options.method || "GET",
    headers,
    signal: options.signal,
    body: options.body ? JSON.stringify(options.body) : undefined
  });

  if (!response.ok) {
    let message = "请求失败";
    try {
      const data = await response.json();
      if (data.error) message = data.error;
      if (data.message) message = data.message;
    } catch {
      message = response.statusText || message;
    }
    const error = new Error(message);
    error.status = response.status;
    throw error;
  }

  if (response.status === 204) return null;
  return response.json();
}

async function uploadFile(file, kind, token) {
  const data = new FormData();
  data.append("file", file);
  data.append("kind", kind);

  const response = await fetch("/api/uploads", {
    method: "POST",
    headers: {
      Accept: "application/json",
      Authorization: `Bearer ${token}`
    },
    body: data
  });

  if (!response.ok) {
    let message = "上传失败";
    try {
      const payload = await response.json();
      if (payload.error) message = payload.error;
      if (payload.message) message = payload.message;
    } catch {
      message = response.statusText || message;
    }
    throw new Error(message);
  }

  return response.json();
}

async function loadFollowingMap(token) {
  const next = {};
  let cursor = "";
  for (let page = 0; page < 20; page++) {
    const data = await apiRequest(relationListPath("following", cursor, 100), { token });
    for (const item of data.items || []) {
      next[item.user_id] = true;
    }
    if (!data.has_more || !data.next_cursor) {
      break;
    }
    cursor = data.next_cursor;
  }
  return next;
}

function relationListPath(tab, cursor = "", limit = 20) {
  const params = new URLSearchParams({ limit: String(limit) });
  if (cursor) {
    params.set("cursor", cursor);
  }
  const resource = tab === "followers" ? "followers" : "following";
  return `/api/users/me/${resource}?${params.toString()}`;
}

function updateSessionRelationCount(session, followingCount) {
  if (!session.user || !Number.isFinite(Number(followingCount))) return;
  session.setAuth(session.token, {
    ...session.user,
    following_count: Number(followingCount),
    followingCount: Number(followingCount)
  });
}

function mapFeedItem(item, feedScene = "timeline", requestID = "") {
  return {
    entry_id: item.entry_id || `${requestID}:${item.video_id}`,
    video_id: item.video_id,
    author_id: item.author_id,
    title: item.title,
    media_url: item.media_url,
    cover_url: item.cover_url,
    like_count: item.like_count,
    comment_count: item.comment_count,
    favorite_count: item.favorite_count,
    liked: Boolean(item.liked),
    favorited: Boolean(item.favorited),
    author: item.author_nickname || `创作者_${item.author_id}`,
    avatar_url: item.author_avatar_url || image.creator,
    description: item.description || "",
    model_name: item.model_name,
    model_params: item.model_params,
    ai_style_tag: item.ai_style_tag,
    origin_video_id: item.origin_video_id,
    feed_scene: feedScene,
    request_id: requestID
  };
}

function isVideoSource(url) {
  return /\.(mp4|webm|ogg|mov)(\?|#|$)/i.test(url || "");
}

function formatMetric(value) {
  const number = Number(value || 0);
  if (number >= 100000000) return `${trimMetric(number / 100000000, number >= 1000000000 ? 0 : 1)}亿`;
  if (number >= 10000) return `${trimMetric(number / 10000, number >= 100000 ? 0 : 1)}万`;
  return String(number);
}

function formatOptionalMetric(value) {
  if (value === undefined || value === null) return "...";
  return formatMetric(value);
}

function trimMetric(value, digits) {
  return value.toFixed(digits).replace(/\.0$/, "");
}

function getFeedTrackStyle(swipe) {
  if (!swipe) {
    return {
      transform: "translate3d(0, 0, 0)"
    };
  }
  const base = swipe.direction === "prev" ? -swipe.height : 0;
  return {
    transform: `translate3d(0, ${base + swipe.offset}px, 0)`,
    transition: swipe.settling ? `transform ${FEED_TRANSITION_MS}ms cubic-bezier(0.16, 1, 0.3, 1)` : "none"
  };
}

function clampSwipeOffset(direction, delta, height) {
  if (direction === "next") {
    return Math.max(-height, Math.min(0, delta));
  }
  return Math.min(height, Math.max(0, delta));
}

function isInteractiveTarget(target) {
  return Boolean(target?.closest?.("button, a, input, textarea, select, details, .comment-panel, [contenteditable='true'], .modal-backdrop"));
}

function formatRelativeTime(value) {
  const time = new Date(value).getTime();
  if (!Number.isFinite(time)) return "";
  const seconds = Math.max(0, Math.floor((Date.now() - time) / 1000));
  if (seconds < 60) return "刚刚";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes} 分钟前`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours} 小时前`;
  const days = Math.floor(hours / 24);
  if (days < 7) return `${days} 天前`;
  return new Date(value).toLocaleDateString("zh-CN");
}

export default App;
