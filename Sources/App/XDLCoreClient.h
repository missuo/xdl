#import <Cocoa/Cocoa.h>

NS_ASSUME_NONNULL_BEGIN

typedef NS_ENUM(NSInteger, XDLCoreState) {
    XDLCoreStateStopped = 0,
    XDLCoreStateStarting,
    XDLCoreStateReady,
};

/// Posted on the main thread when the core process state changes.
extern NSNotificationName const XDLCoreStateDidChangeNotification;
/// Posted on the main thread for every event from GET /events.
/// userInfo: @{ @"type": NSString, @"data": id (may be NSNull) }
extern NSNotificationName const XDLCoreEventNotification;
/// Posted on the main thread for every core log line. userInfo: @{ @"line": NSString }
extern NSNotificationName const XDLCoreLogLineNotification;

/// Owns the xdl-core child process and the localhost HTTP control API:
/// spawns the Go binary, discovers its ephemeral port via the api-info file,
/// polls /events, and fans everything out as NSNotifications.
@interface XDLCoreClient : NSObject

@property (nonatomic, readonly) XDLCoreState state;
@property (nonatomic, readonly) NSArray<NSString *> *logLines;

- (void)start;
- (void)stop;

/// Append an app-side line to the Logs page (core lines arrive automatically).
- (void)logAppLine:(NSString *)line;

/// JSON request helpers. Completion is always delivered on the main thread.
/// Non-2xx responses arrive as NSError with the server's "error" message.
- (void)get:(NSString *)path completion:(void (^)(id _Nullable json, NSError *_Nullable error))completion;
- (void)post:(NSString *)path body:(nullable NSDictionary *)body
  completion:(void (^_Nullable)(id _Nullable json, NSError *_Nullable error))completion;
/// POST with a per-request timeout (seconds); 0 uses the session default.
- (void)post:(NSString *)path body:(nullable NSDictionary *)body timeout:(NSTimeInterval)timeout
  completion:(void (^_Nullable)(id _Nullable json, NSError *_Nullable error))completion;

@end

NS_ASSUME_NONNULL_END
