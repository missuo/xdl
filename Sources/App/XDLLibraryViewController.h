#import <Cocoa/Cocoa.h>
#import "XDLPageViewController.h"

NS_ASSUME_NONNULL_BEGIN

@class XDLCoreClient;

/// Saved accounts stored in the core's SQLite database.
@interface XDLLibraryViewController : XDLPageViewController

@property (nonatomic, copy, nullable) void (^onOpenAccount)(NSString *username, NSString *mediaType);
@property (nonatomic, copy, nullable) void (^onRefetchAccount)(NSString *username, NSString *mediaType);

- (instancetype)initWithClient:(XDLCoreClient *)client;

@end

NS_ASSUME_NONNULL_END
