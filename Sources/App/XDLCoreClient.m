#import "XDLCoreClient.h"
#import <Security/Security.h>

NSNotificationName const XDLCoreStateDidChangeNotification = @"XDLCoreStateDidChangeNotification";
NSNotificationName const XDLCoreEventNotification = @"XDLCoreEventNotification";
NSNotificationName const XDLCoreLogLineNotification = @"XDLCoreLogLineNotification";

static NSString *const kErrorDomain = @"XDLCoreClient";
static const NSUInteger kMaxLogLines = 2000;

@interface XDLCoreClient ()
@property (nonatomic, readwrite) XDLCoreState state;
@property (nonatomic, nullable) NSTask *task;
@property (nonatomic) NSString *secret;
@property (nonatomic) NSString *apiInfoPath;
@property (nonatomic, nullable) NSString *baseURL;
@property (nonatomic) NSURLSession *session;
@property (nonatomic, nullable) NSTimer *discoveryTimer;
@property (nonatomic, nullable) NSTimer *eventTimer;
@property (nonatomic) long long eventCursor;
@property (nonatomic) BOOL eventRequestInFlight;
@property (nonatomic) NSMutableArray<NSString *> *logBuffer;
@property (nonatomic) NSInteger restartAttempts;
@property (nonatomic) BOOL stopping;
@end

@implementation XDLCoreClient

- (instancetype)init {
    self = [super init];
    if (self) {
        _logBuffer = [NSMutableArray array];
        _eventCursor = -1;
        NSURLSessionConfiguration *config = [NSURLSessionConfiguration ephemeralSessionConfiguration];
        // Extraction responses can take minutes (xtractor paginates upstream).
        config.timeoutIntervalForRequest = 600;
        config.timeoutIntervalForResource = 3600;
        _session = [NSURLSession sessionWithConfiguration:config];
    }
    return self;
}

- (NSArray<NSString *> *)logLines {
    @synchronized (self.logBuffer) {
        return [self.logBuffer copy];
    }
}

- (void)logAppLine:(NSString *)line {
    [self appendLogLine:[@"[app] " stringByAppendingString:line ?: @""]];
}

#pragma mark - Lifecycle

- (NSString *)corePath {
    NSString *override = NSProcessInfo.processInfo.environment[@"XDL_CORE_PATH"];
    if (override.length > 0) {
        return override;
    }
    return [NSBundle.mainBundle pathForResource:@"xdl-core" ofType:nil];
}

- (void)start {
    if (self.task.isRunning) {
        return;
    }
    self.stopping = NO;

    NSString *corePath = [self corePath];
    if (corePath.length == 0 || ![NSFileManager.defaultManager isExecutableFileAtPath:corePath]) {
        [self appendLogLine:@"[app] xdl-core binary not found in app bundle"];
        self.state = XDLCoreStateStopped;
        [self notifyState];
        return;
    }

    NSString *supportDir = [NSSearchPathForDirectoriesInDomains(NSApplicationSupportDirectory, NSUserDomainMask, YES).firstObject
        stringByAppendingPathComponent:@"XDL"];
    [NSFileManager.defaultManager createDirectoryAtPath:supportDir
                            withIntermediateDirectories:YES
                                             attributes:nil
                                                  error:nil];
    self.apiInfoPath = [supportDir stringByAppendingPathComponent:@"api-info.json"];
    [NSFileManager.defaultManager removeItemAtPath:self.apiInfoPath error:nil];

    uint8_t randomBytes[32];
    if (SecRandomCopyBytes(kSecRandomDefault, sizeof(randomBytes), randomBytes) != errSecSuccess) {
        arc4random_buf(randomBytes, sizeof(randomBytes));
    }
    NSMutableString *secret = [NSMutableString stringWithCapacity:64];
    for (size_t i = 0; i < sizeof(randomBytes); i++) {
        [secret appendFormat:@"%02x", randomBytes[i]];
    }
    self.secret = secret;

    NSTask *task = [[NSTask alloc] init];
    task.executableURL = [NSURL fileURLWithPath:corePath];
    task.arguments = @[ @"-api-info", self.apiInfoPath ];
    NSMutableDictionary *env = [NSProcessInfo.processInfo.environment mutableCopy];
    env[@"XDL_API_SECRET"] = self.secret;
    task.environment = env;

    NSPipe *stdoutPipe = [NSPipe pipe];
    NSPipe *stderrPipe = [NSPipe pipe];
    task.standardOutput = stdoutPipe;
    task.standardError = stderrPipe;
    [self attachLogReader:stdoutPipe];
    [self attachLogReader:stderrPipe];

    __weak typeof(self) weakSelf = self;
    task.terminationHandler = ^(NSTask *finished) {
        dispatch_async(dispatch_get_main_queue(), ^{
            [weakSelf coreDidTerminateWithStatus:finished.terminationStatus];
        });
    };

    NSError *error = nil;
    if (![task launchAndReturnError:&error]) {
        [self appendLogLine:[NSString stringWithFormat:@"[app] failed to launch xdl-core: %@", error.localizedDescription]];
        self.state = XDLCoreStateStopped;
        [self notifyState];
        return;
    }

    self.task = task;
    self.state = XDLCoreStateStarting;
    self.baseURL = nil;
    self.eventCursor = -1;
    [self notifyState];
    [self appendLogLine:@"[app] xdl-core launched, waiting for api-info"];

    self.discoveryTimer = [NSTimer scheduledTimerWithTimeInterval:0.2
                                                           target:self
                                                         selector:@selector(discoveryTick)
                                                         userInfo:nil
                                                          repeats:YES];
}

- (void)attachLogReader:(NSPipe *)pipe {
    __weak typeof(self) weakSelf = self;
    __block NSMutableData *pending = [NSMutableData data];
    pipe.fileHandleForReading.readabilityHandler = ^(NSFileHandle *handle) {
        NSData *data = handle.availableData;
        if (data.length == 0) {
            handle.readabilityHandler = nil;
            return;
        }
        [pending appendData:data];
        // Split off complete lines.
        while (true) {
            NSRange newline = [pending rangeOfData:[NSData dataWithBytes:"\n" length:1]
                                           options:0
                                             range:NSMakeRange(0, pending.length)];
            if (newline.location == NSNotFound) {
                break;
            }
            NSData *lineData = [pending subdataWithRange:NSMakeRange(0, newline.location)];
            [pending replaceBytesInRange:NSMakeRange(0, newline.location + 1) withBytes:NULL length:0];
            NSString *line = [[NSString alloc] initWithData:lineData encoding:NSUTF8StringEncoding];
            if (line.length > 0) {
                [weakSelf appendLogLine:line];
            }
        }
    };
}

- (void)appendLogLine:(NSString *)line {
    @synchronized (self.logBuffer) {
        [self.logBuffer addObject:line];
        if (self.logBuffer.count > kMaxLogLines) {
            [self.logBuffer removeObjectsInRange:NSMakeRange(0, self.logBuffer.count - kMaxLogLines)];
        }
    }
    dispatch_async(dispatch_get_main_queue(), ^{
        [NSNotificationCenter.defaultCenter postNotificationName:XDLCoreLogLineNotification
                                                          object:self
                                                        userInfo:@{ @"line": line }];
    });
}

- (void)discoveryTick {
    NSData *data = [NSData dataWithContentsOfFile:self.apiInfoPath];
    if (!data) {
        return;
    }
    NSDictionary *info = [NSJSONSerialization JSONObjectWithData:data options:0 error:nil];
    NSString *address = info[@"address"];
    if (![info isKindOfClass:NSDictionary.class] || address.length == 0) {
        return;
    }
    [self.discoveryTimer invalidate];
    self.discoveryTimer = nil;
    self.baseURL = [NSString stringWithFormat:@"http://%@", address];

    __weak typeof(self) weakSelf = self;
    [self get:@"/ping" completion:^(id json, NSError *error) {
        __strong typeof(weakSelf) self = weakSelf;
        if (!self || self.stopping) return;
        if (error) {
            [self appendLogLine:[NSString stringWithFormat:@"[app] core ping failed: %@", error.localizedDescription]];
            return;
        }
        self.restartAttempts = 0;
        self.state = XDLCoreStateReady;
        [self notifyState];
        [self appendLogLine:[NSString stringWithFormat:@"[app] core ready at %@", self.baseURL]];
        [self startEventPolling];
    }];
}

- (void)startEventPolling {
    [self.eventTimer invalidate];
    self.eventTimer = [NSTimer scheduledTimerWithTimeInterval:0.4
                                                       target:self
                                                     selector:@selector(eventTick)
                                                     userInfo:nil
                                                      repeats:YES];
    self.eventTimer.tolerance = 0.1;
}

- (void)eventTick {
    if (self.state != XDLCoreStateReady || self.eventRequestInFlight) {
        return;
    }
    self.eventRequestInFlight = YES;
    NSString *path = [NSString stringWithFormat:@"/events?cursor=%lld", self.eventCursor];
    __weak typeof(self) weakSelf = self;
    [self get:path completion:^(id json, NSError *error) {
        __strong typeof(weakSelf) self = weakSelf;
        if (!self) return;
        self.eventRequestInFlight = NO;
        if (error || ![json isKindOfClass:NSDictionary.class]) {
            return;
        }
        NSNumber *cursor = json[@"cursor"];
        if ([cursor isKindOfClass:NSNumber.class]) {
            self.eventCursor = cursor.longLongValue;
        }
        for (NSDictionary *event in json[@"events"]) {
            if (![event isKindOfClass:NSDictionary.class]) continue;
            NSString *type = event[@"type"];
            if (![type isKindOfClass:NSString.class]) continue;
            [NSNotificationCenter.defaultCenter postNotificationName:XDLCoreEventNotification
                                                              object:self
                                                            userInfo:@{
                @"type": type,
                @"data": event[@"data"] ?: NSNull.null,
            }];
        }
    }];
}

- (void)coreDidTerminateWithStatus:(int)status {
    [self appendLogLine:[NSString stringWithFormat:@"[app] xdl-core exited with status %d", status]];
    [self.discoveryTimer invalidate];
    self.discoveryTimer = nil;
    [self.eventTimer invalidate];
    self.eventTimer = nil;
    self.task = nil;
    self.baseURL = nil;
    self.state = XDLCoreStateStopped;
    [self notifyState];

    if (!self.stopping && self.restartAttempts < 3) {
        self.restartAttempts += 1;
        [self appendLogLine:[NSString stringWithFormat:@"[app] restarting core (attempt %ld)", (long)self.restartAttempts]];
        __weak typeof(self) weakSelf = self;
        dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(2 * NSEC_PER_SEC)),
                       dispatch_get_main_queue(), ^{
            [weakSelf start];
        });
    }
}

- (void)stop {
    self.stopping = YES;
    [self.discoveryTimer invalidate];
    self.discoveryTimer = nil;
    [self.eventTimer invalidate];
    self.eventTimer = nil;

    NSTask *task = self.task;
    if (task.isRunning) {
        [self post:@"/shutdown" body:nil completion:nil];
        // Give the graceful path a moment, then make sure.
        dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(1.5 * NSEC_PER_SEC)),
                       dispatch_get_main_queue(), ^{
            if (task.isRunning) {
                [task terminate];
            }
        });
    }
}

- (void)notifyState {
    [NSNotificationCenter.defaultCenter postNotificationName:XDLCoreStateDidChangeNotification object:self];
}

#pragma mark - HTTP

- (void)get:(NSString *)path completion:(void (^)(id, NSError *))completion {
    [self requestWithMethod:@"GET" path:path body:nil timeout:0 completion:completion];
}

- (void)post:(NSString *)path body:(NSDictionary *)body completion:(void (^)(id, NSError *))completion {
    [self requestWithMethod:@"POST" path:path body:body timeout:0 completion:completion];
}

- (void)post:(NSString *)path body:(NSDictionary *)body timeout:(NSTimeInterval)timeout
  completion:(void (^)(id, NSError *))completion {
    [self requestWithMethod:@"POST" path:path body:body timeout:timeout completion:completion];
}

- (void)requestWithMethod:(NSString *)method
                     path:(NSString *)path
                     body:(NSDictionary *)body
                  timeout:(NSTimeInterval)timeout
               completion:(void (^)(id, NSError *))completion {
    void (^finish)(id, NSError *) = ^(id json, NSError *error) {
        if (!completion) return;
        dispatch_async(dispatch_get_main_queue(), ^{
            completion(json, error);
        });
    };

    if (self.baseURL.length == 0) {
        finish(nil, [NSError errorWithDomain:kErrorDomain code:1 userInfo:@{
            NSLocalizedDescriptionKey: @"Core is not running",
        }]);
        return;
    }

    NSURL *url = [NSURL URLWithString:[self.baseURL stringByAppendingString:path]];
    NSMutableURLRequest *request = [NSMutableURLRequest requestWithURL:url];
    request.HTTPMethod = method;
    if (timeout > 0) {
        request.timeoutInterval = timeout;
    }
    [request setValue:[@"Bearer " stringByAppendingString:self.secret] forHTTPHeaderField:@"Authorization"];
    if (body) {
        NSError *encodeError = nil;
        request.HTTPBody = [NSJSONSerialization dataWithJSONObject:body options:0 error:&encodeError];
        if (encodeError) {
            finish(nil, encodeError);
            return;
        }
        [request setValue:@"application/json" forHTTPHeaderField:@"Content-Type"];
    }

    [[self.session dataTaskWithRequest:request
                     completionHandler:^(NSData *data, NSURLResponse *response, NSError *error) {
        if (error) {
            finish(nil, error);
            return;
        }
        id json = nil;
        if (data.length > 0) {
            json = [NSJSONSerialization JSONObjectWithData:data options:0 error:nil];
        }
        NSInteger status = ((NSHTTPURLResponse *)response).statusCode;
        if (status < 200 || status >= 300) {
            NSString *message = [NSString stringWithFormat:@"HTTP %ld", (long)status];
            if ([json isKindOfClass:NSDictionary.class] && [json[@"error"] isKindOfClass:NSString.class]) {
                message = json[@"error"];
            }
            finish(nil, [NSError errorWithDomain:kErrorDomain code:status userInfo:@{
                NSLocalizedDescriptionKey: message,
            }]);
            return;
        }
        finish(json, nil);
    }] resume];
}

@end
